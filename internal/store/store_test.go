package store_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
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
		got, err := s.Show(ctx, "a/b", audit.ActionUse)
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

// withRecorder gives s a Recorder that audits only the audited names.
func withRecorder(t *testing.T, s *store.Store, audited ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	s.Recorder = &audit.Recorder{
		Path:    path,
		Via:     "get",
		Now:     func() time.Time { return time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC) },
		Audited: func(entry string) bool { return slices.Contains(audited, entry) },
		Callers: func() []audit.Process { return nil },
	}
	return path
}

func readEvents(t *testing.T, path string) []audit.Event {
	t.Helper()
	events, err := audit.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func TestShowOfAuditedEntryRecordsOneEventWithTheAction(t *testing.T) {
	env, s := newStore(t)
	env.Insert(t, "github", "ghp-value")
	path := withRecorder(t, s, "github")

	if _, err := s.Show(context.Background(), "github", audit.ActionUse); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, path)
	if len(events) != 1 || events[0].Entry != "github" || events[0].Action != audit.ActionUse {
		t.Fatalf("events = %+v", events)
	}
}

func TestShowFailsClosedWhenAuditLogCannotBeWritten(t *testing.T) {
	env, s := newStore(t)
	env.Insert(t, "github", "ghp-value")
	recorder := env.Runner()
	s.Runner = recorder
	path := withRecorder(t, s, "github")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	value, err := s.Show(context.Background(), "github", audit.ActionUse)
	if err == nil || !strings.HasPrefix(err.Error(), "audit log: ") || value != nil {
		t.Fatalf("value = %q, err = %v", value, err)
	}
	if len(recorder.Argv()) != 0 {
		t.Fatalf("pass ran despite the audit failure: %v", recorder.Argv())
	}
}

func TestShowOfEntryThatIsNotAuditedRecordsNothing(t *testing.T) {
	env, s := newStore(t)
	env.Insert(t, "aws", "aws-value")
	path := withRecorder(t, s, "github")

	if _, err := s.Show(context.Background(), "aws", audit.ActionUse); err != nil {
		t.Fatal(err)
	}
	if events := readEvents(t, path); len(events) != 0 {
		t.Fatalf("events = %+v", events)
	}
}

func TestInsertOfNewAuditedEntryWithoutGitRecordsAddedWrite(t *testing.T) {
	_, s := newStore(t)
	path := withRecorder(t, s, "github")

	if err := s.Insert(context.Background(), "github", []byte("v")); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, path)
	if len(events) != 1 || events[0].Action != audit.ActionWrite || events[0].Detail != "added" {
		t.Fatalf("events = %+v", events)
	}
}

func TestRemoveOfAuditedEntryRecordsDelete(t *testing.T) {
	env, s := newStore(t)
	env.Insert(t, "github", "v")
	path := withRecorder(t, s, "github")

	if err := s.Remove(context.Background(), "github"); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, path)
	if len(events) != 1 || events[0].Action != audit.ActionDelete {
		t.Fatalf("events = %+v", events)
	}
}

func TestCopyOfAuditedEntryFailsClosedWhenAuditLogCannotBeWritten(t *testing.T) {
	env, s := newStore(t)
	env.Insert(t, "github", "v")
	recorder := env.Runner()
	s.Runner = recorder
	path := withRecorder(t, s, "github")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	err := s.Copy(context.Background(), "github")
	if err == nil || !strings.HasPrefix(err.Error(), "audit log: ") {
		t.Fatalf("err = %v", err)
	}
	if len(recorder.Argv()) != 0 {
		t.Fatalf("pass ran despite the audit failure: %v", recorder.Argv())
	}
}
