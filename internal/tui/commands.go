package tui

import (
	"context"

	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/opcli"
	"github.com/atavares-klaviyo/pass-vault/internal/store"
	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

// Messages that tea.Cmd functions deliver back to Update.
type (
	loadedMsg struct {
		rows   []syncer.StatusRow
		state  syncer.State
		opOK   bool
		mapErr string
	}
	historyMsg   struct{ revisions []store.Revision }
	revisionsMsg struct {
		revisions []store.Revision
		tags      []string
	}
	revealMsg struct {
		name  string
		value string
	}
	tickMsg         struct{ at time.Time }
	syncDoneMsg     struct{ added, unchanged, skipped int }
	checkDoneMsg    struct{ rows []syncer.StatusRow }
	rollbackDoneMsg struct{ hash string }
	mutatedMsg      struct{ note string }
	itemsMsg        struct{ items []opcli.Item }
	fieldsMsg       struct{ item opcli.Item }
	appendedMsg     struct{ name string }
	editorDoneMsg   struct{ err error }
	errMsg          struct{ err error }
)

// loadCmd reads mapping, state, store and op health in one command.
func loadCmd(deps Deps) tea.Cmd {
	return func() tea.Msg {
		m, state, mapErr := loadMappingState(deps)
		names, err := deps.Store.List()
		if err != nil {
			return errMsg{err}
		}
		return loadedMsg{
			rows:   syncer.Status(m, state, names),
			state:  state,
			opOK:   opHealthy(deps.Source),
			mapErr: errString(mapErr),
		}
	}
}

func loadMappingState(deps Deps) (mapping.Mapping, syncer.State, error) {
	m, mapErr := deps.Mapping()
	state, err := deps.State()
	if err != nil && mapErr == nil {
		mapErr = err
	}
	return m, state, mapErr
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// opHealthy reports whether op can reach an account, when it supports it.
func opHealthy(source syncer.Source) bool {
	client, ok := source.(interface {
		SignedIn(ctx context.Context) error
	})
	if !ok {
		return true
	}
	return client.SignedIn(context.Background()) == nil
}

// revealCmd decrypts name and schedules the re-mask tick.
func revealCmd(st *store.Store, name string) tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			value, err := st.Show(context.Background(), name)
			if err != nil {
				return errMsg{err}
			}
			return revealMsg{name: name, value: string(value)}
		},
		tea.Tick(revealDuration, func(at time.Time) tea.Msg {
			return tickMsg{at: at}
		}),
	)
}

// historyCmd loads the git history of one entry.
func historyCmd(st *store.Store, name string) tea.Cmd {
	return func() tea.Msg {
		revisions, err := st.History(context.Background(), name, 20)
		if err != nil {
			return errMsg{err}
		}
		return historyMsg{revisions}
	}
}

// revisionsCmd loads all commits and sync tags for the history view.
func revisionsCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		revisions, err := st.History(context.Background(), "", 50)
		if err != nil {
			return errMsg{err}
		}
		tags, err := st.SyncTags(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return revisionsMsg{revisions, tags}
	}
}

// checkCmd runs a live check and returns fresh status rows.
func checkCmd(deps Deps) tea.Cmd {
	return func() tea.Msg {
		m, state, mapErr := loadMappingState(deps)
		if mapErr != nil {
			return errMsg{mapErr}
		}
		s := syncer.Syncer{Source: deps.Source, Vault: deps.Store, State: &state, Now: deps.Now}
		if _, err := s.Check(context.Background(), m, nil); err != nil {
			return errMsg{err}
		}
		names, err := deps.Store.List()
		if err != nil {
			return errMsg{err}
		}
		return checkDoneMsg{syncer.Status(m, state, names)}
	}
}

// insertCmd stores a new entry, then reports it so the model reloads.
func insertCmd(st *store.Store, name, value string) tea.Cmd {
	return func() tea.Msg {
		bytes := []byte(value)
		defer clear(bytes)
		if err := st.Insert(context.Background(), name, bytes); err != nil {
			return errMsg{err}
		}
		return mutatedMsg{"added " + name}
	}
}

// removeCmd deletes an entry from the mapping, the store and the sync
// state; otherwise sync re-creates it or it stays listed as an orphan.
func removeCmd(deps Deps, name string) tea.Cmd {
	return func() tea.Msg {
		if m, err := deps.Mapping(); err == nil {
			if _, ok := m.Find(name); ok {
				if err := mapping.RemoveSecret(deps.MappingPath, name); err != nil {
					return errMsg{err}
				}
			}
		}
		if err := deps.Store.Remove(context.Background(), name); err != nil {
			return errMsg{err}
		}
		state, err := deps.State()
		if err != nil {
			return errMsg{err}
		}
		if _, ok := state.Entries[name]; ok {
			delete(state.Entries, name)
			if err := deps.SaveState(state); err != nil {
				return errMsg{err}
			}
		}
		return mutatedMsg{"deleted " + name}
	}
}

// copyCmd puts an entry on the clipboard; pass clears it after 45 s.
func copyCmd(st *store.Store, name string) tea.Cmd {
	return func() tea.Msg {
		if err := st.Copy(context.Background(), name); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

// rollbackCmd restores the store to revision, then reports it.
func rollbackCmd(st *store.Store, revision string) tea.Cmd {
	return func() tea.Msg {
		hash, err := st.Rollback(context.Background(), revision, "")
		if err != nil {
			return errMsg{err}
		}
		return rollbackDoneMsg{hash}
	}
}

// planCmd previews a rollback of revision.
func planCmd(st *store.Store, revision string) tea.Cmd {
	return func() tea.Msg {
		restored, removed, err := st.RollbackPlan(context.Background(), revision, "")
		if err != nil {
			return errMsg{err}
		}
		return planMsg{revision: revision, restored: restored, removed: removed}
	}
}

// itemsCmd lists 1Password items for the add picker.
func itemsCmd(items ItemSource) tea.Cmd {
	return func() tea.Msg {
		list, err := items.ListItems(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return itemsMsg{list}
	}
}

// fieldsCmd loads one item's field labels and references, never values.
func fieldsCmd(items ItemSource, id string) tea.Cmd {
	return func() tea.Msg {
		item, err := items.ItemFields(context.Background(), id)
		if err != nil {
			return errMsg{err}
		}
		return fieldsMsg{item}
	}
}

// appendCmd adds one secret to the mapping file.
func appendCmd(path string, secret mapping.Secret) tea.Cmd {
	return func() tea.Msg {
		if err := mapping.AppendSecret(path, secret); err != nil {
			return errMsg{err}
		}
		return appendedMsg{secret.Pass}
	}
}
