package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

// keyMap lists the keys that work right now, for the help footer; full
// adds the keys only the full help (?) lists.
type keyMap struct {
	bindings []key.Binding
	full     []key.Binding
}

func (m Model) keyMap() keyMap {
	return keyMap{bindings: bindings(m.activeKeys(false)), full: bindings(m.activeKeys(true))}
}

// activeKeys picks hints for the open prompt, conflict or view.
func (m Model) activeKeys(full bool) [][2]string {
	if m.prompt != promptNone {
		return [][2]string{{"enter", "confirm"}, {"esc", "cancel"}}
	}
	if m.conflict.get() != nil {
		return [][2]string{{"o", "overwrite"}, {"s", "skip"}, {"enter", "show both"}, {"esc", "stop sync"}}
	}
	if len(m.pickerItems) > 0 && m.picker == pickerItems {
		return [][2]string{{"type", "search"}, {"←/→", "vault"}, {"↑/↓", "move"}, {"enter", "select"}, {"esc", "close"}}
	}
	if len(m.pickerItems) > 0 {
		return [][2]string{{"↑/↓", "move"}, {"enter", "select"}, {"esc", "close"}}
	}
	return append(m.viewKeys(full), [2]string{"tab", "next view"}, [2]string{"?", "help"}, [2]string{"q", "quit"})
}

func (m Model) viewKeys(full bool) [][2]string {
	switch m.active {
	case viewVault:
		return vaultKeys(full)
	case viewSync:
		return [][2]string{{"s", "sync"}}
	case viewHistory:
		return m.historyKeys()
	case viewMapping:
		return [][2]string{{"n", "new"}, {"e", "edit"}}
	case viewEnv:
		return [][2]string{{"n", "new"}, {"a", "add/rename env"}, {"space", "on/off"}, {"A", "audit on/off"}, {"d", "remove env"}}
	case viewAudit:
		return [][2]string{{"j/k", "move"}, {"f", "filter entry"}, {"a", m.auditShowAllHint()}}
	}
	return nil
}

// vaultKeys lists the Vault keys; new, delete and refresh only in full help.
func vaultKeys(full bool) [][2]string {
	keys := [][2]string{{"enter", "show/hide"}, {"c", "copy"}, {"h", "history"}, {"p", "holders"}, {"s", "sync"}}
	if full {
		keys = append(keys, [2]string{"n", "new"}, [2]string{"d", "delete"}, [2]string{"r", "refresh"})
	}
	return keys
}

func (m Model) historyKeys() [][2]string {
	if m.historyEntry != "" {
		return [][2]string{{"enter", "roll back entry"}, {"f", "filter entry"}}
	}
	return [][2]string{{"enter", "roll back to"}, {"f", "filter entry"}}
}

func (m Model) auditShowAllHint() string {
	if m.auditShowAll {
		return "hide sync/compare"
	}
	return "show sync/compare"
}

func bindings(keys [][2]string) []key.Binding {
	result := make([]key.Binding, len(keys))
	for i, pair := range keys {
		result[i] = key.NewBinding(key.WithKeys(pair[0]), key.WithHelp(pair[0], pair[1]))
	}
	return result
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding { return k.bindings }

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.full} }

var _ help.KeyMap = keyMap{}
