package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

// keyMap lists the keys that work right now, for the help footer.
type keyMap struct {
	bindings []key.Binding
}

func (m Model) keyMap() keyMap {
	return keyMap{bindings: bindings(m.activeKeys())}
}

// activeKeys picks hints for the open prompt, conflict or view.
func (m Model) activeKeys() [][2]string {
	if m.prompt != promptNone {
		return [][2]string{{"enter", "confirm"}, {"esc", "cancel"}}
	}
	if m.conflict.get() != nil {
		return [][2]string{{"o", "overwrite"}, {"s", "skip"}, {"enter", "show both"}, {"esc", "stop sync"}}
	}
	if len(m.pickerItems) > 0 {
		return [][2]string{{"↑/↓", "move"}, {"enter", "select"}, {"esc", "close"}}
	}
	return append(m.viewKeys(), [2]string{"tab", "next view"}, [2]string{"?", "help"}, [2]string{"q", "quit"})
}

func (m Model) viewKeys() [][2]string {
	switch m.active {
	case viewVault:
		return [][2]string{{"enter", "show/hide"}, {"c", "copy"}, {"n", "new"},
			{"d", "delete"}, {"s", "sync"}, {"r", "refresh"}}
	case viewSync:
		return [][2]string{{"s", "sync"}}
	case viewHistory:
		return [][2]string{{"enter", "roll back to"}}
	case viewMapping:
		return [][2]string{{"n", "new"}, {"e", "edit"}}
	}
	return nil
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
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.bindings} }

var _ help.KeyMap = keyMap{}
