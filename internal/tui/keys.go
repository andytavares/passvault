package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

// keyMap lists the global keys for the help footer.
type keyMap struct {
	bindings []key.Binding
}

func (m Model) keyMap() keyMap {
	keys := [][2]string{
		{"1-4/tab", "view"},
		{"?", "help"},
		{"q", "quit"},
	}
	switch m.active {
	case viewVault:
		keys = append(keys, [2]string{"s", "sync"}, [2]string{"c", "check"},
			[2]string{"r", "reveal"}, [2]string{"y", "copy"},
			[2]string{"n", "new"}, [2]string{"d", "delete"})
	case viewHistory:
		keys = append(keys, [2]string{"R", "rollback"})
	case viewMapping:
		keys = append(keys, [2]string{"e", "edit"}, [2]string{"a", "add"})
	}
	bindings := make([]key.Binding, len(keys))
	for i, pair := range keys {
		bindings[i] = key.NewBinding(key.WithKeys(pair[0]), key.WithHelp(pair[0], pair[1]))
	}
	return keyMap{bindings: bindings}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding { return k.bindings }

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.bindings} }

var _ help.KeyMap = keyMap{}
