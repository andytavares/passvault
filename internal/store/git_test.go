package store_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/store"
	"github.com/atavares-klaviyo/pass-vault/internal/testenv"
)

// gitStore wraps Store with failing-fast helpers so each test stays small.
type gitStore struct {
	t *testing.T
	*store.Store
}

func newGitStore(t *testing.T) gitStore {
	t.Helper()
	env := testenv.New(t).WithStore(t).WithGit(t)
	return gitStore{t, &store.Store{Runner: env.Runner(), Dir: env.Paths.StoreDir}}
}

func (g gitStore) insert(name, value string) {
	g.t.Helper()
	if err := g.Insert(context.Background(), name, []byte(value)); err != nil {
		g.t.Fatal(err)
	}
}

func (g gitStore) show(name string) string {
	g.t.Helper()
	got, err := g.Show(context.Background(), name)
	if err != nil {
		g.t.Fatal(err)
	}
	return string(got)
}

func (g gitStore) head() string {
	g.t.Helper()
	head, err := g.Head(context.Background())
	if err != nil {
		g.t.Fatal(err)
	}
	return head
}

func (g gitStore) commit(path, message string) {
	g.t.Helper()
	ctx := context.Background()
	if _, err := g.Runner.Run(ctx, store.GitCmd(g.Dir, "add", path)); err != nil {
		g.t.Fatal(err)
	}
	if _, err := g.Runner.Run(ctx, store.GitCmd(g.Dir, "commit", "-m", message)); err != nil {
		g.t.Fatal(err)
	}
}

func TestIsGitRepo(t *testing.T) {
	env, s := newStore(t)
	if s.IsGitRepo() {
		t.Fatal("store without .git reported as repo")
	}
	env.WithGit(t)
	if !s.IsGitRepo() {
		t.Fatal("store with .git not reported as repo")
	}
}

func TestDirtyTracksWorktree(t *testing.T) {
	s := newGitStore(t)
	ctx := context.Background()
	dirty, err := s.Dirty(ctx)
	if err != nil || dirty {
		t.Fatalf("clean store: dirty=%v err=%v", dirty, err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "untracked.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirty, err = s.Dirty(ctx)
	if err != nil || !dirty {
		t.Fatalf("untracked file: dirty=%v err=%v", dirty, err)
	}
}

func TestHeadAndTag(t *testing.T) {
	s := newGitStore(t)
	ctx := context.Background()
	if head := s.head(); len(head) != 40 {
		t.Fatalf("Head = %q", head)
	}
	if err := s.Tag(ctx, "sync-20261025T110204"); err != nil {
		t.Fatal(err)
	}
	tags, err := s.SyncTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != "sync-20261025T110204" {
		t.Fatalf("SyncTags = %v", tags)
	}
}

func TestSyncTagsNewestFirst(t *testing.T) {
	s := newGitStore(t)
	ctx := context.Background()
	s.insert("e", "v1")
	if err := s.Tag(ctx, "sync-20261025T110204"); err != nil {
		t.Fatal(err)
	}
	s.insert("e", "v2")
	if err := s.Tag(ctx, "sync-20261026T110204"); err != nil {
		t.Fatal(err)
	}
	if err := s.Tag(ctx, "unrelated"); err != nil {
		t.Fatal(err)
	}
	tags, err := s.SyncTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"sync-20261026T110204", "sync-20261025T110204"}
	if strings.Join(tags, ",") != strings.Join(want, ",") {
		t.Fatalf("SyncTags = %v, want %v", tags, want)
	}
}

func TestHistoryFiltersByEntry(t *testing.T) {
	s := newGitStore(t)
	s.insert("one", "v1")
	s.insert("two", "v1")
	s.insert("one", "v2")
	revisions, err := s.History(context.Background(), "one", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 {
		t.Fatalf("History(one) = %d revisions, want 2", len(revisions))
	}
	for _, revision := range revisions {
		assertCompleteRevision(t, revision, "one.gpg")
	}
}

func assertCompleteRevision(t *testing.T, revision store.Revision, file string) {
	t.Helper()
	if len(revision.Hash) != 40 || revision.Short == "" || revision.Subject == "" || revision.Time.IsZero() {
		t.Fatalf("incomplete revision: %+v", revision)
	}
	if !slices.Contains(revision.Files, file) {
		t.Fatalf("revision %s missing %s in Files %v", revision.Short, file, revision.Files)
	}
}

func TestHistoryAllAndLimit(t *testing.T) {
	s := newGitStore(t)
	s.insert("one", "v1")
	s.insert("two", "v1")
	ctx := context.Background()
	all, err := s.History(ctx, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 3 {
		t.Fatalf("History(\"\") = %d revisions, want at least 3", len(all))
	}
	limited, err := s.History(ctx, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("History limit 1 = %d revisions", len(limited))
	}
}

func TestHistoryIncludesTags(t *testing.T) {
	s := newGitStore(t)
	ctx := context.Background()
	s.insert("one", "v1")
	if err := s.Tag(ctx, "sync-20261025T110204"); err != nil {
		t.Fatal(err)
	}
	revisions, err := s.History(ctx, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 || len(revisions[0].Tags) != 1 || revisions[0].Tags[0] != "sync-20261025T110204" {
		t.Fatalf("revisions = %+v", revisions)
	}
}

func TestRollbackRestoresOldContentAndRemovesNewer(t *testing.T) {
	s := newGitStore(t)
	s.insert("kept", "old")
	head := s.head()
	s.insert("kept", "new")
	s.insert("added", "new")
	hash, err := s.Rollback(context.Background(), head, "")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("rollback returned no commit hash")
	}
	if got := s.show("kept"); got != "old" {
		t.Fatalf("kept = %q", got)
	}
	if s.Exists("added") {
		t.Fatal("added entry survived whole-store rollback")
	}
}

func TestRollbackEntryTouchesOnlyThatFile(t *testing.T) {
	s := newGitStore(t)
	s.insert("target", "old")
	head := s.head()
	s.insert("target", "new")
	s.insert("other", "untouched")
	if _, err := s.Rollback(context.Background(), head, "target"); err != nil {
		t.Fatal(err)
	}
	if got := s.show("target"); got != "old" {
		t.Fatalf("target = %q", got)
	}
	if got := s.show("other"); got != "untouched" {
		t.Fatalf("other = %q", got)
	}
}

func TestRollbackRefusesWhenDirty(t *testing.T) {
	s := newGitStore(t)
	head := s.head()
	if err := os.WriteFile(filepath.Join(s.Dir, "untracked.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rollback(context.Background(), head, ""); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("err = %v", err)
	}
}

func TestRollbackRefusesOnGpgIDChange(t *testing.T) {
	s := newGitStore(t)
	head := s.head()
	path := filepath.Join(s.Dir, ".gpg-id")
	if err := os.WriteFile(path, []byte("DIFFERENTKEY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.commit(".gpg-id", "rekey")
	if _, err := s.Rollback(context.Background(), head, ""); err == nil || !strings.Contains(err.Error(), "different key") {
		t.Fatalf("err = %v", err)
	}
}

func TestRollbackNoOpReturnsEmpty(t *testing.T) {
	s := newGitStore(t)
	hash, err := s.Rollback(context.Background(), s.head(), "")
	if err != nil {
		t.Fatal(err)
	}
	if hash != "" {
		t.Fatalf("no-op rollback returned %q", hash)
	}
}

func TestRollbackPlan(t *testing.T) {
	s := newGitStore(t)
	s.insert("kept", "old")
	head := s.head()
	s.insert("kept", "new")
	s.insert("added", "new")
	restored, removed, err := s.RollbackPlan(context.Background(), head, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(restored, ",") != "kept" {
		t.Fatalf("restored = %v", restored)
	}
	if strings.Join(removed, ",") != "added" {
		t.Fatalf("removed = %v", removed)
	}
}
