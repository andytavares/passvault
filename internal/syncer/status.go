package syncer

import (
	"sort"
	"time"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

// StatusKind classifies one pass entry for `pvault status`.
type StatusKind string

const (
	KindMapped        StatusKind = "mapped"
	KindMappedMissing StatusKind = "mapped-missing"
	KindUnmanaged     StatusKind = "unmanaged"
	KindOrphan        StatusKind = "orphan"
)

// StatusRow is one line of `pvault status`. It needs no decrypt.
type StatusRow struct {
	Name     string
	Kind     StatusKind
	Env      string
	Scope    mapping.Scope
	SyncedAt time.Time
	Tag      string
}

// Status classifies every mapped and vault entry against the state.
func Status(m mapping.Mapping, state State, vaultNames []string) []StatusRow {
	rows := mappedRows(m, state, vaultNames)
	rows = append(rows, vaultRows(m, state, vaultNames)...)
	unmapped := State{Entries: map[string]EntryState{}}
	for name, entry := range state.Entries {
		if _, ok := m.Find(name); !ok {
			unmapped.Entries[name] = entry
		}
	}
	rows = append(rows, missingOrphanRows(unmapped, vaultNames)...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

// mappedRows covers mapping secrets: Mapped when in the vault, else
// MappedMissing. Local entries (no Ref) are left to vaultRows.
func mappedRows(m mapping.Mapping, state State, vaultNames []string) []StatusRow {
	var rows []StatusRow
	for _, secret := range m.Secrets {
		if secret.Ref == "" {
			continue
		}
		row := StatusRow{Name: secret.Pass, Env: secret.Env, Scope: secret.Scope, Kind: KindMappedMissing}
		if containsName(vaultNames, secret.Pass) {
			row.Kind = KindMapped
		}
		if entry, ok := state.Entries[secret.Pass]; ok {
			row.SyncedAt, row.Tag = entry.SyncedAt, entry.Tag
		}
		rows = append(rows, row)
	}
	return rows
}

// vaultRows covers vault entries outside the mapping: Orphan when the state
// synced them once, else Unmanaged (made by hand, never touched). A local
// mapping entry counts as outside the mapping and keeps its Env and Scope.
func vaultRows(m mapping.Mapping, state State, vaultNames []string) []StatusRow {
	var rows []StatusRow
	for _, name := range vaultNames {
		secret, found := m.Find(name)
		if found && secret.Ref != "" {
			continue
		}
		row := StatusRow{Name: name, Kind: KindUnmanaged, Env: secret.Env, Scope: secret.Scope}
		if entry, ok := state.Entries[name]; ok {
			row.Kind, row.SyncedAt, row.Tag = KindOrphan, entry.SyncedAt, entry.Tag
		}
		rows = append(rows, row)
	}
	return rows
}

// missingOrphanRows covers state entries gone from the vault; the caller
// already excluded mapping secrets.
func missingOrphanRows(state State, vaultNames []string) []StatusRow {
	var rows []StatusRow
	for name, entry := range state.Entries {
		if containsName(vaultNames, name) {
			continue
		}
		rows = append(rows, StatusRow{Name: name, Kind: KindOrphan, SyncedAt: entry.SyncedAt, Tag: entry.Tag})
	}
	return rows
}
