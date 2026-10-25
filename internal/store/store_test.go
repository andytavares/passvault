package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/store"
	"github.com/atavares-klaviyo/pass-vault/internal/testenv"
)

func newStore(t *testing.T) (*testenv.Env, *store.Store) {
	t.Helper()
	env := testenv.New(t).WithStore(t)
	return env, &store.Store{Runner: env.Runner(), Dir: env.Paths.StoreDir}
}

func TestInsertShowRoundTripsExactBytes(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	for _, value := range []string{"line1\nline2\n", "no trailing newline"} {
		if err := s.Insert(ctx, "a/b", []byte(value)); err != nil {
			t.Fatal(err)
		}
		got, err := s.Show(ctx, "a/b")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != value {
			t.Fatalf("got %q, want %q", got, value)
		}
	}
	_ = env
}

func TestInsertNeverPutsValueInArgv(t *testing.T) {
	env, s := newStore(t)
	recorder := env.Runner()
	s.Runner = recorder
	secret := "s3cr3t-value-never-in-argv"
	if err := s.Insert(context.Background(), "entry", []byte(secret)); err != nil {
		t.Fatal(err)
	}
	for _, argv := range recorder.Argv() {
		if strings.Contains(argv, secret) {
			t.Fatalf("secret leaked into argv: %q", argv)
		}
	}
}

func TestExists(t *testing.T) {
	_, s := newStore(t)
	if s.Exists("missing") {
		t.Fatal("missing entry reported as existing")
	}
	if err := s.Insert(context.Background(), "present", []byte("v")); err != nil {
		t.Fatal(err)
	}
	if !s.Exists("present") {
		t.Fatal("present entry reported as missing")
	}
}

func TestRemove(t *testing.T) {
	_, s := newStore(t)
	ctx := context.Background()
	if err := s.Insert(ctx, "gone", []byte("v")); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(ctx, "gone"); err != nil {
		t.Fatal(err)
	}
	if s.Exists("gone") {
		t.Fatal("entry still exists after Remove")
	}
}

func TestListSkipsGitAndFindsNested(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	for _, name := range []string{"zeta", "a/b", "a/c"} {
		if err := s.Insert(ctx, name, []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	env.WithGit(t)
	names, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a/b", "a/c", "zeta"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("List = %v, want %v", names, want)
	}
}

func TestGpgID(t *testing.T) {
	env, s := newStore(t)
	id, err := s.GpgID()
	if err != nil {
		t.Fatal(err)
	}
	if id != env.Fingerprint {
		t.Fatalf("GpgID = %q, want %q", id, env.Fingerprint)
	}
}

func TestGpgIDReadsFirstLineOnly(t *testing.T) {
	_, s := newStore(t)
	path := filepath.Join(s.Dir, ".gpg-id")
	if err := os.WriteFile(path, []byte("FIRST\nSECOND\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := s.GpgID()
	if err != nil {
		t.Fatal(err)
	}
	if id != "FIRST" {
		t.Fatalf("GpgID = %q", id)
	}
}
