package syncer_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atavares-klaviyo/pass-vault/internal/execx"
	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/opcli"
	"github.com/atavares-klaviyo/pass-vault/internal/store"
	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
	"github.com/atavares-klaviyo/pass-vault/internal/testenv"
)

var fixedNow = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

type scriptDecider struct {
	decisions []syncer.Decision
	calls     int
}

func (d *scriptDecider) Decide(_ context.Context, _ syncer.Conflict) (syncer.Decision, error) {
	if d.calls >= len(d.decisions) {
		return syncer.DecisionSkip, nil
	}
	decision := d.decisions[d.calls]
	d.calls++
	return decision, nil
}

func newSyncer(t *testing.T, env *testenv.Env, decider syncer.Decider) (*syncer.Syncer, *execx.Recorder) {
	t.Helper()
	recorder := env.Runner()
	return &syncer.Syncer{
		Source:  &opcli.Client{Runner: recorder},
		Vault:   &store.Store{Runner: recorder, Dir: env.Paths.StoreDir},
		State:   &syncer.State{Entries: map[string]syncer.EntryState{}},
		Now:     func() time.Time { return fixedNow },
		Decider: decider,
	}, recorder
}

func newEnv(t *testing.T) *testenv.Env {
	t.Helper()
	return testenv.New(t).WithStore(t).WithGit(t)
}

func mappingOf(secrets ...mapping.Secret) mapping.Mapping {
	return mapping.Mapping{Version: 1, Secrets: secrets}
}

func secret(pass, ref string) mapping.Secret {
	return mapping.Secret{Pass: pass, Ref: ref, Scope: mapping.ScopeNone}
}

func commitCount(t *testing.T, env *testenv.Env) string {
	t.Helper()
	result, err := execx.OS{}.Run(context.Background(), store.GitCmd(env.Paths.StoreDir, "rev-list", "--count", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(result.Stdout))
}

func TestSyncInsertsNewEntryWithExactBytes(t *testing.T) {
	env := newEnv(t)
	value := "line1\nline2\n"
	env.SetOp(t, "op://Private/Item/field", value)
	s, _ := newSyncer(t, env, nil)
	report, err := s.Sync(context.Background(), mappingOf(secret("a/b", "op://Private/Item/field")), syncer.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := env.Show(t, "a/b"); got != value {
		t.Fatalf("got %q, want %q", got, value)
	}
	if report.Added != 1 {
		t.Fatalf("Added = %d, want 1", report.Added)
	}
}

func TestSyncUnchangedMakesNoCommitAndNoTag(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.SetOp(t, "op://Private/Item/field", "value")
	s, _ := newSyncer(t, env, nil)
	m := mappingOf(secret("a/b", "op://Private/Item/field"))
	if _, err := s.Sync(ctx, m, syncer.SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	before := commitCount(t, env)
	report, err := s.Sync(ctx, m, syncer.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if after := commitCount(t, env); after != before {
		t.Fatalf("commit count changed: %s -> %s", before, after)
	}
	if report.Tag != "" {
		t.Fatalf("unexpected tag %q", report.Tag)
	}
	if report.Unchanged != 1 {
		t.Fatalf("Unchanged = %d, want 1", report.Unchanged)
	}
}

func TestSyncDiffersDeciderSkipKeepsVaultBytes(t *testing.T) {
	env := newEnv(t)
	env.Insert(t, "a/b", "old-value")
	env.SetOp(t, "op://Private/Item/field", "new-value")
	s, _ := newSyncer(t, env, &scriptDecider{decisions: []syncer.Decision{syncer.DecisionSkip}})
	report, err := s.Sync(context.Background(), mappingOf(secret("a/b", "op://Private/Item/field")), syncer.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := env.Show(t, "a/b"); got != "old-value" {
		t.Fatalf("vault bytes changed: %q", got)
	}
	if report.Skipped != 1 || report.Differs() != 1 {
		t.Fatalf("Skipped = %d, Differs = %d, want 1 and 1", report.Skipped, report.Differs())
	}
}

func TestSyncDiffersDeciderOverwriteWritesNewBytes(t *testing.T) {
	env := newEnv(t)
	env.Insert(t, "a/b", "old-value")
	env.SetOp(t, "op://Private/Item/field", "new-value")
	s, _ := newSyncer(t, env, &scriptDecider{decisions: []syncer.Decision{syncer.DecisionOverwrite}})
	report, err := s.Sync(context.Background(), mappingOf(secret("a/b", "op://Private/Item/field")), syncer.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := env.Show(t, "a/b"); got != "new-value" {
		t.Fatalf("got %q, want new-value", got)
	}
	if report.Updated != 1 {
		t.Fatalf("Updated = %d, want 1", report.Updated)
	}
}

func TestSyncDiffersDeciderQuitStopsButTagsDone(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.Insert(t, "one", "old-one")
	env.Insert(t, "two", "old-two")
	env.SetOp(t, "op://Private/Item/one", "new-one")
	env.SetOp(t, "op://Private/Item/two", "new-two")
	m := mappingOf(secret("one", "op://Private/Item/one"), secret("two", "op://Private/Item/two"))
	s, _ := newSyncer(t, env, &scriptDecider{decisions: []syncer.Decision{syncer.DecisionOverwrite, syncer.DecisionQuit}})
	report, err := s.Sync(ctx, m, syncer.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Quit {
		t.Fatal("Quit = false, want true")
	}
	if got := env.Show(t, "one"); got != "new-one" {
		t.Fatalf("one = %q, want new-one", got)
	}
	if got := env.Show(t, "two"); got != "old-two" {
		t.Fatalf("two = %q, want untouched old-two", got)
	}
	assertOneSyncTag(t, env, report.Tag)
}

func assertOneSyncTag(t *testing.T, env *testenv.Env, want string) {
	t.Helper()
	if want == "" {
		t.Fatal("no tag although one entry was overwritten")
	}
	vault := &store.Store{Runner: env.Runner(), Dir: env.Paths.StoreDir}
	tags, err := vault.SyncTags(context.Background())
	if err != nil || len(tags) != 1 || tags[0] != want {
		t.Fatalf("SyncTags = %v, %v; want [%s]", tags, err, want)
	}
}

func TestSyncOrphanReportedNotDeletedWithoutPrune(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.Insert(t, "gone", "value")
	s, _ := newSyncer(t, env, nil)
	s.State.Entries["gone"] = syncer.EntryState{SyncedAt: fixedNow, Tag: "sync-1"}
	report, err := s.Sync(ctx, mappingOf(), syncer.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Orphans != 1 || report.Pruned != 0 {
		t.Fatalf("Orphans = %d, Pruned = %d, want 1 and 0", report.Orphans, report.Pruned)
	}
	if !s.Vault.Exists("gone") {
		t.Fatal("orphan deleted without Prune")
	}
	if _, ok := s.State.Entries["gone"]; !ok {
		t.Fatal("orphan dropped from state without Prune")
	}
}

func TestSyncPruneDeletesOrphanButNeverHandmadeEntry(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.Insert(t, "gone", "value")
	env.Insert(t, "handmade", "mine")
	s, _ := newSyncer(t, env, nil)
	s.State.Entries["gone"] = syncer.EntryState{SyncedAt: fixedNow, Tag: "sync-1"}
	report, err := s.Sync(ctx, mappingOf(), syncer.SyncOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Pruned != 1 {
		t.Fatalf("Pruned = %d, want 1", report.Pruned)
	}
	if s.Vault.Exists("gone") {
		t.Fatal("orphan still present after Prune")
	}
	if !s.Vault.Exists("handmade") {
		t.Fatal("handmade entry was removed")
	}
	if _, ok := s.State.Entries["gone"]; ok {
		t.Fatal("pruned orphan still in state")
	}
}

func TestSyncDirtyStoreErrors(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	stray := filepath.Join(env.Paths.StoreDir, "stray.gpg")
	if err := os.WriteFile(stray, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (execx.OS{}).Run(ctx, store.GitCmd(env.Paths.StoreDir, "add", "stray.gpg")); err != nil {
		t.Fatal(err)
	}
	env.SetOp(t, "op://Private/Item/field", "value")
	s, _ := newSyncer(t, env, nil)
	_, err := s.Sync(ctx, mappingOf(secret("a/b", "op://Private/Item/field")), syncer.SyncOptions{})
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("err = %v, want uncommitted changes", err)
	}
	if s.Vault.Exists("a/b") {
		t.Fatal("sync wrote to a dirty store")
	}
}

func TestSyncDryRunOnDirtyStoreWritesNothing(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	stray := filepath.Join(env.Paths.StoreDir, "stray.gpg")
	if err := os.WriteFile(stray, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	env.SetOp(t, "op://Private/Item/field", "value")
	s, _ := newSyncer(t, env, nil)
	report, err := s.Sync(ctx, mappingOf(secret("a/b", "op://Private/Item/field")), syncer.SyncOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Added != 1 {
		t.Fatalf("Added = %d, want 1", report.Added)
	}
	if s.Vault.Exists("a/b") {
		t.Fatal("dry run wrote an entry")
	}
	if report.Tag != "" {
		t.Fatalf("dry run tagged: %q", report.Tag)
	}
}

func TestSyncNeverLeaksSecretIntoArgvOrState(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	value := "s3cr3t-never-leaked-12345"
	env.SetOp(t, "op://Private/Item/field", value)
	s, recorder := newSyncer(t, env, nil)
	if _, err := s.Sync(ctx, mappingOf(secret("a/b", "op://Private/Item/field")), syncer.SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, argv := range recorder.Argv() {
		if strings.Contains(argv, value) {
			t.Fatalf("secret in argv: %q", argv)
		}
	}
	statePath := filepath.Join(env.Paths.StateDir, "state.json")
	if err := syncer.SaveState(statePath, *s.State); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(value))
	for _, leaked := range []string{value, hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString([]byte(value))} {
		if strings.Contains(string(data), leaked) {
			t.Fatalf("state file contains secret material: %s", data)
		}
	}
}

func TestSyncSourceFailureMarksEntryAndContinues(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.SetOp(t, "op://Private/Item/good", "good-value")
	m := mappingOf(secret("bad", "op://Private/Item/missing"), secret("good", "op://Private/Item/good"))
	s, _ := newSyncer(t, env, nil)
	report, err := s.Sync(ctx, m, syncer.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Failed != 1 || report.Added != 1 || report.ExitCode() != 2 {
		t.Fatalf("Failed = %d, Added = %d, ExitCode = %d, want 1, 1 and 2", report.Failed, report.Added, report.ExitCode())
	}
	if got := env.Show(t, "good"); got != "good-value" {
		t.Fatalf("good = %q, want good-value", got)
	}
	assertFailedEntry(t, report.Entries)
}

func assertFailedEntry(t *testing.T, entries []syncer.Entry) {
	t.Helper()
	var failedEntry *syncer.Entry
	for i := range entries {
		if entries[i].Action == syncer.ActionFailed {
			failedEntry = &entries[i]
		}
	}
	if failedEntry == nil || failedEntry.Err == nil {
		t.Fatalf("no failed entry with error in %+v", entries)
	}
	if strings.Contains(failedEntry.Err.Error(), "good-value") {
		t.Fatalf("error leaks a secret: %v", failedEntry.Err)
	}
}

func TestSyncSkipConflictsLeavesDiffers(t *testing.T) {
	env := newEnv(t)
	env.Insert(t, "a/b", "old-value")
	env.SetOp(t, "op://Private/Item/field", "new-value")
	s, _ := newSyncer(t, env, nil)
	report, err := s.Sync(context.Background(), mappingOf(secret("a/b", "op://Private/Item/field")), syncer.SyncOptions{SkipConflicts: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Differs() != 1 || report.Skipped != 1 {
		t.Fatalf("Differs = %d, Skipped = %d, want 1 and 1", report.Differs(), report.Skipped)
	}
	if got := env.Show(t, "a/b"); got != "old-value" {
		t.Fatalf("vault bytes changed: %q", got)
	}
}

func TestSyncOnlyFiltersByPassName(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.SetOp(t, "op://Private/Item/one", "one-value")
	env.SetOp(t, "op://Private/Item/two", "two-value")
	m := mappingOf(secret("one", "op://Private/Item/one"), secret("two", "op://Private/Item/two"))
	s, _ := newSyncer(t, env, nil)
	report, err := s.Sync(ctx, m, syncer.SyncOptions{Only: []string{"one"}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Added != 1 || len(report.Entries) != 1 {
		t.Fatalf("Added = %d, Entries = %d, want 1 and 1", report.Added, len(report.Entries))
	}
	if !s.Vault.Exists("one") || s.Vault.Exists("two") {
		t.Fatal("Only filter not applied")
	}
}

func TestSyncTagUsesInjectedNow(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.SetOp(t, "op://Private/Item/field", "value")
	s, _ := newSyncer(t, env, nil)
	report, err := s.Sync(ctx, mappingOf(secret("a/b", "op://Private/Item/field")), syncer.SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Tag != "sync-20261025T120000" {
		t.Fatalf("Tag = %q, want sync-20261025T120000", report.Tag)
	}
	vault := &store.Store{Runner: env.Runner(), Dir: env.Paths.StoreDir}
	tags, err := vault.SyncTags(ctx)
	if err != nil || len(tags) != 1 || tags[0] != "sync-20261025T120000" {
		t.Fatalf("SyncTags = %v, %v", tags, err)
	}
	if s.State.LastTag != "sync-20261025T120000" {
		t.Fatalf("LastTag = %q", s.State.LastTag)
	}
	if entry := s.State.Entries["a/b"]; !entry.SyncedAt.Equal(fixedNow) || entry.Tag != report.Tag {
		t.Fatalf("state entry = %+v", entry)
	}
}

func TestCheckClassifiesActions(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	env.Insert(t, "same", "value")
	env.Insert(t, "diff", "old")
	env.SetOp(t, "op://Private/Item/same", "value")
	env.SetOp(t, "op://Private/Item/diff", "new")
	env.SetOp(t, "op://Private/Item/add", "added")
	m := mappingOf(
		secret("same", "op://Private/Item/same"),
		secret("diff", "op://Private/Item/diff"),
		secret("add", "op://Private/Item/add"),
	)
	s, _ := newSyncer(t, env, nil)
	entries, err := s.Check(ctx, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]syncer.Action{}
	for _, entry := range entries {
		actions[entry.Secret.Pass] = entry.Action
	}
	want := map[string]syncer.Action{"same": syncer.ActionUnchanged, "diff": syncer.ActionDiffers, "add": syncer.ActionAdd}
	for name, action := range want {
		if actions[name] != action {
			t.Fatalf("%s: %s, want %s", name, actions[name], action)
		}
	}
}
