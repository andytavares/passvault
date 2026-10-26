package syncer_test

import (
	"testing"

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
