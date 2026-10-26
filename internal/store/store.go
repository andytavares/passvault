// Package store wraps the pass store: entry reads and writes, and the git
// history that powers rollback. Secrets pass only through stdin/stdout.
package store

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/atavares-klaviyo/pass-vault/internal/execx"
)

// Store is one pass store directory.
type Store struct {
	Runner execx.Runner
	Dir    string
}

// passCmd builds a pass command with PASSWORD_STORE_DIR pinned to Dir.
func (s *Store) passCmd(args ...string) execx.Cmd {
	return execx.Cmd{
		Name: "pass",
		Args: args,
		Env:  []string{"PASSWORD_STORE_DIR=" + s.Dir},
	}
}

// GitCmd builds a git command against Dir. Tests reuse it for direct commits.
func GitCmd(dir string, args ...string) execx.Cmd {
	return execx.Cmd{Name: "git", Args: append([]string{"-C", dir}, args...)}
}

// Show returns the exact decrypted bytes of name.
func (s *Store) Show(ctx context.Context, name string) ([]byte, error) {
	result, err := s.Runner.Run(ctx, s.passCmd("show", name))
	if err != nil {
		return nil, err
	}
	return result.Stdout, nil
}

// Exists reports whether name.gpg is on disk.
func (s *Store) Exists(name string) bool {
	_, err := os.Stat(filepath.Join(s.Dir, name+".gpg"))
	return err == nil
}

// Insert writes value as entry name, overwriting. Value goes via stdin only.
func (s *Store) Insert(ctx context.Context, name string, value []byte) error {
	cmd := s.passCmd("insert", "-m", "-f", name)
	cmd.Stdin = value
	_, err := s.Runner.Run(ctx, cmd)
	return err
}

// Remove deletes entry name.
func (s *Store) Remove(ctx context.Context, name string) error {
	_, err := s.Runner.Run(ctx, s.passCmd("rm", "-f", name))
	return err
}

// List returns every entry name, sorted, without the .gpg suffix.
func (s *Store) List() ([]string, error) {
	var names []string
	err := filepath.WalkDir(s.Dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".gpg") {
			rel, err := filepath.Rel(s.Dir, path)
			if err != nil {
				return err
			}
			names = append(names, strings.TrimSuffix(rel, ".gpg"))
		}
		return nil
	})
	sort.Strings(names)
	return names, err
}

// Copy puts entry name on the clipboard; pass clears it after 45 s.
func (s *Store) Copy(ctx context.Context, name string) error {
	_, err := s.Runner.Run(ctx, s.passCmd("show", "-c", name))
	return err
}

// GpgID returns the first line of Dir/.gpg-id.
func (s *Store) GpgID() (string, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, ".gpg-id"))
	if err != nil {
		return "", err
	}
	first, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(first), nil
}
