package syncer_test

import (
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

func TestStatusClassifiesFourKinds(t *testing.T) {
	m := mappingOf(secret("mapped", "op://Private/Item/a"), secret("missing", "op://Private/Item/b"))
	state := syncer.State{Entries: map[string]syncer.EntryState{
		"mapped": {SyncedAt: fixedNow, Tag: "sync-1"},
		"orphan": {SyncedAt: fixedNow, Tag: "sync-1"},
	}}
	rows := syncer.Status(m, state, []string{"mapped", "orphan", "unmanaged"})
	kinds := map[string]syncer.StatusKind{}
	for _, row := range rows {
		kinds[row.Name] = row.Kind
	}
	want := map[string]syncer.StatusKind{
		"mapped":    syncer.KindMapped,
		"missing":   syncer.KindMappedMissing,
		"unmanaged": syncer.KindUnmanaged,
		"orphan":    syncer.KindOrphan,
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for name, kind := range want {
		if kinds[name] != kind {
			t.Fatalf("%s: %s, want %s", name, kinds[name], kind)
		}
	}
	mapped := rows[0]
	if mapped.Name != "mapped" || mapped.Tag != "sync-1" || !mapped.SyncedAt.Equal(fixedNow) {
		t.Fatalf("mapped row = %+v", mapped)
	}
}

func TestStatusLocalEntryIsUnmanagedRowWhenInVault(t *testing.T) {
	m := mappingOf(
		mapping.Secret{Pass: "local", Env: "LOCAL", Scope: mapping.ScopeGlobal},
		mapping.Secret{Pass: "absent", Env: "ABSENT", Scope: mapping.ScopeGlobal},
	)
	rows := syncer.Status(m, syncer.State{Entries: map[string]syncer.EntryState{}}, []string{"local"})
	want := syncer.StatusRow{Name: "local", Kind: syncer.KindUnmanaged, Env: "LOCAL", Scope: mapping.ScopeGlobal}
	if len(rows) != 1 || rows[0] != want {
		t.Fatalf("rows = %+v, want [%+v]", rows, want)
	}
}
