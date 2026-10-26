package syncer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

func TestLoadStateMissingReturnsEmpty(t *testing.T) {
	state, err := syncer.LoadState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 0 || state.LastTag != "" {
		t.Fatalf("state = %+v, want empty", state)
	}
}

func TestSaveLoadStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	want := syncer.State{
		LastTag: "sync-20261025T120000",
		Entries: map[string]syncer.EntryState{
			"a/b": {SyncedAt: fixedNow, Tag: "sync-20261025T120000"},
		},
	}
	if err := syncer.SaveState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := syncer.LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastTag != want.LastTag || !got.Entries["a/b"].SyncedAt.Equal(want.Entries["a/b"].SyncedAt) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	assertModes(t, path)
}

func assertModes(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o, want 600", info.Mode().Perm())
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dir.Mode().Perm() != 0o700 {
		t.Fatalf("state dir mode = %o, want 700", dir.Mode().Perm())
	}
}

func TestSaveStateStoresNoSecretMaterial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := syncer.State{Entries: map[string]syncer.EntryState{
		"a/b": {SyncedAt: time.Now(), Tag: "sync-1"},
	}}
	if err := syncer.SaveState(path, state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"value", "hash", "sha", "secret"} {
		if strings.Contains(string(data), `"`+key+`"`) {
			t.Fatalf("state file mentions %q: %s", key, data)
		}
	}
}
