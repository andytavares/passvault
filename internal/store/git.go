package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Revision is one commit in the store's history.
type Revision struct {
	Hash    string
	Short   string
	Subject string
	Time    time.Time
	Tags    []string
	Files   []string
}

// IsGitRepo reports whether Dir holds a git repository.
func (s *Store) IsGitRepo() bool {
	info, err := os.Stat(filepath.Join(s.Dir, ".git"))
	return err == nil && info.IsDir()
}

// GitInit runs pass git init on the store.
func (s *Store) GitInit(ctx context.Context) error {
	_, err := s.Runner.Run(ctx, s.passCmd("git", "init"))
	return err
}

// Dirty reports whether the worktree has staged, unstaged or untracked files.
func (s *Store) Dirty(ctx context.Context) (bool, error) {
	result, err := s.Runner.Run(ctx, GitCmd(s.Dir, "status", "--porcelain"))
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(result.Stdout))) > 0, nil
}

// Head returns the full hash of HEAD.
func (s *Store) Head(ctx context.Context) (string, error) {
	result, err := s.Runner.Run(ctx, GitCmd(s.Dir, "rev-parse", "HEAD"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(result.Stdout)), nil
}

// Tag creates an (unannotated) tag at HEAD.
func (s *Store) Tag(ctx context.Context, name string) error {
	_, err := s.Runner.Run(ctx, GitCmd(s.Dir, "tag", name))
	return err
}

// SyncTags returns every sync-* tag, newest first. creatordate has second
// resolution, so ties fall back to the tag names, which embed a timestamp.
func (s *Store) SyncTags(ctx context.Context) ([]string, error) {
	result, err := s.Runner.Run(ctx, GitCmd(s.Dir, "tag", "--list", "sync-*", "--sort=-creatordate"))
	if err != nil {
		return nil, err
	}
	tags := splitLines(string(result.Stdout))
	sort.Sort(sort.Reverse(sort.StringSlice(tags)))
	return tags, nil
}

// historyFormat separates fields with \x1f and records with \x1e.
const historyFormat = "%H%x1f%h%x1f%s%x1f%aI%x1f%D%x1e"

// History lists commits, newest first. With name set, only commits touching
// name.gpg; with limit > 0, at most limit commits.
func (s *Store) History(ctx context.Context, name string, limit int) ([]Revision, error) {
	args := []string{"log", "--decorate=short", "--format=" + historyFormat, "--name-only"}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}
	if name != "" {
		args = append(args, "--", name+".gpg")
	}
	result, err := s.Runner.Run(ctx, GitCmd(s.Dir, args...))
	if err != nil {
		return nil, err
	}
	return parseHistory(string(result.Stdout)), nil
}

// parseHistory reads git log --name-only output: one header line per commit
// (fields split by \x1f, ended by \x1e), then one file per line.
func parseHistory(output string) []Revision {
	var revisions []Revision
	current := -1
	for _, line := range strings.Split(output, "\n") {
		if strings.ContainsRune(line, '\x1f') {
			revisions = append(revisions, parseRevisionHeader(strings.TrimSuffix(line, "\x1e")))
			current = len(revisions) - 1
			continue
		}
		if line != "" && current >= 0 {
			revisions[current].Files = append(revisions[current].Files, line)
		}
	}
	return revisions
}

func parseRevisionHeader(header string) Revision {
	fields := strings.SplitN(header, "\x1f", 5)
	instant, _ := time.Parse(time.RFC3339, fields[3])
	return Revision{
		Hash:    fields[0],
		Short:   fields[1],
		Subject: fields[2],
		Time:    instant,
		Tags:    parseTags(fields[4]),
	}
}

// parseTags extracts tag names from git's %D decoration ("HEAD -> main, tag: X").
func parseTags(decoration string) []string {
	var tags []string
	for _, part := range strings.Split(decoration, ",") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(part), "tag: "); ok {
			tags = append(tags, name)
		}
	}
	return tags
}

// Rollback restores the store, or one entry, to revision with a new commit.
// It returns the new short hash, or "" when nothing changed.
func (s *Store) Rollback(ctx context.Context, revision, entry string) (string, error) {
	if dirty, err := s.Dirty(ctx); err != nil || dirty {
		return "", fmt.Errorf("store is dirty; commit or discard changes first")
	}
	if err := s.checkGpgID(ctx, revision); err != nil {
		return "", err
	}
	if err := s.restore(ctx, revision, entry); err != nil {
		return "", err
	}
	if dirty, err := s.Dirty(ctx); err != nil || !dirty {
		return "", err
	}
	return s.commitRollback(ctx, revision, entry)
}

// checkGpgID refuses rollback when the store was encrypted to a different
// key at revision.
func (s *Store) checkGpgID(ctx context.Context, revision string) error {
	result, err := s.Runner.Run(ctx, GitCmd(s.Dir, "show", revision+":.gpg-id"))
	if err != nil {
		return err
	}
	then := strings.TrimSpace(string(result.Stdout))
	now, err := s.GpgID()
	if err != nil {
		return err
	}
	if then != now {
		return fmt.Errorf("store was encrypted to a different key at %s", revision)
	}
	return nil
}

func (s *Store) restore(ctx context.Context, revision, entry string) error {
	paths := []string{"--", "."}
	if entry != "" {
		paths = []string{"--", entry + ".gpg"}
	}
	args := append([]string{"restore", "--source=" + revision, "--staged", "--worktree"}, paths...)
	if _, err := s.Runner.Run(ctx, GitCmd(s.Dir, args...)); err != nil {
		return err
	}
	if entry != "" && !s.Exists(entry) && s.trackedAt(ctx, revision, entry) {
		_, err := s.Runner.Run(ctx, GitCmd(s.Dir, "rm", "-q", entry+".gpg"))
		return err
	}
	return nil
}

// trackedAt reports whether entry.gpg exists at revision.
func (s *Store) trackedAt(ctx context.Context, revision, entry string) bool {
	_, err := s.Runner.Run(ctx, GitCmd(s.Dir, "cat-file", "-e", revision+":"+entry+".gpg"))
	return err == nil
}

func (s *Store) commitRollback(ctx context.Context, revision, entry string) (string, error) {
	message := "pvault rollback to " + revision
	if entry != "" {
		message = fmt.Sprintf("pvault rollback %s to %s", entry, revision)
	}
	if _, err := s.Runner.Run(ctx, GitCmd(s.Dir, "commit", "-m", message)); err != nil {
		return "", err
	}
	head, err := s.Head(ctx)
	if err != nil {
		return "", err
	}
	if len(head) > 7 {
		head = head[:7]
	}
	return head, nil
}

// RollbackPlan previews Rollback: entries restore would change, and entries
// added after revision it would remove. Paths come back without .gpg.
func (s *Store) RollbackPlan(ctx context.Context, revision, entry string) (restored, removed []string, err error) {
	args := []string{"diff", "--name-status", revision, "--", "."}
	if entry != "" {
		args[len(args)-1] = entry + ".gpg"
	}
	result, err := s.Runner.Run(ctx, GitCmd(s.Dir, args...))
	if err != nil {
		return nil, nil, err
	}
	return parsePlan(string(result.Stdout))
}

func parsePlan(output string) (restored, removed []string, err error) {
	for _, line := range splitLines(output) {
		status, path, _ := strings.Cut(line, "\t")
		if path == ".gpg-id" {
			continue
		}
		name := strings.TrimSuffix(path, ".gpg")
		switch status {
		case "A":
			removed = append(removed, name)
		case "M", "D":
			restored = append(restored, name)
		}
	}
	return restored, removed, nil
}

func splitLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
