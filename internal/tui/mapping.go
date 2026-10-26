package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/opcli"
)

// updateMapping handles keys in the mapping view.
func (m Model) updateMapping(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if next, cmd, ok := m.updatePicker(msg); ok {
		return next, cmd
	}
	switch msg.String() {
	case "up", "k":
		if m.mapCursor > 0 {
			m.mapCursor--
		}
	case "down", "j":
		if m.mapCursor < len(m.mapping.Secrets)-1 {
			m.mapCursor++
		}
	case "e":
		return m.openEditor()
	case "n":
		m.picker, m.pickerCursor = pickerItems, 0
		return m, itemsCmd(m.deps.Items)
	}
	return m, nil
}

// openEditor runs $EDITOR on the mapping file, then reloads it.
func (m Model) openEditor() (tea.Model, tea.Cmd) {
	if m.deps.Editor == nil {
		m.err = errors.New("$EDITOR is not set")
		return m, nil
	}
	cmd := m.deps.Editor(m.deps.MappingPath)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{err}
	})
}

// applyEditorDone reloads and re-validates the mapping after the editor.
func (m Model) applyEditorDone(msg editorDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	m.notice = "mapping reloaded"
	return m, loadCmd(m.deps)
}

// updatePicker handles keys while the add-from-1Password picker is open. It
// reports handled=false when no picker is active.
func (m Model) updatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if len(m.pickerItems) == 0 {
		return m, nil, false
	}
	switch msg.String() {
	case "up", "k":
		m.movePicker(-1)
	case "down", "j":
		m.movePicker(1)
	case "enter":
		return m.pickCurrent()
	case "esc":
		m.pickerItems, m.pickerFields = nil, nil
	}
	return m, nil, true
}

func (m *Model) movePicker(delta int) {
	if m.picker == pickerItems {
		m.pickerCursor = clampIndex(m.pickerCursor+delta, len(m.pickerItems))
		return
	}
	m.fieldCursor = clampIndex(m.fieldCursor+delta, len(m.pickerFields))
}

func clampIndex(index, length int) int {
	if index < 0 {
		return 0
	}
	if index >= length {
		return max(0, length-1)
	}
	return index
}

// pickCurrent advances the picker: item → fields → name prompt.
func (m Model) pickCurrent() (tea.Model, tea.Cmd, bool) {
	if m.picker == pickerItems {
		item := m.pickerItems[m.pickerCursor]
		m.picker = pickerFields
		return m, fieldsCmd(m.deps.Items, item.ID), true
	}
	if len(m.pickerFields) == 0 {
		return m, nil, true
	}
	m.pickerRef = m.pickerFields[m.fieldCursor].Reference
	m.prompt, m.input = promptAddName, slugify(m.pickerItems[m.pickerCursor].Title)
	return m, nil, true
}

// mappingView renders mapping entries and the picker when open.
func (m Model) mappingView() string {
	lines := []string{m.styles.title.Render("Mapping") + m.styles.dim.Render(" ─ "+m.deps.MappingPath)}
	if m.mapErrors != "" {
		lines = append(lines, m.styles.bad.Render(m.mapErrors))
	}
	lines = append(lines, m.secretLines()...)
	if len(m.pickerItems) > 0 {
		lines = append(lines, "", m.pickerView())
	}
	return strings.Join(lines, "\n")
}

func (m Model) secretLines() []string {
	if len(m.mapping.Secrets) == 0 {
		return []string{m.styles.dim.Render("no entries; press n to add from 1Password")}
	}
	var lines []string
	for i, secret := range m.mapping.Secrets {
		lines = append(lines, fmt.Sprintf("%s %-24s %s", cursorMark(i == m.mapCursor), secret.Pass, secret.Ref))
	}
	return lines
}

// pickerView renders the item or field list. It never shows field values.
func (m Model) pickerView() string {
	if m.picker == pickerItems {
		return m.itemPickerView()
	}
	return m.fieldPickerView()
}

func (m Model) itemPickerView() string {
	lines := []string{m.styles.dim.Render("Add from 1Password · pick an item")}
	for i, item := range m.pickerItems {
		lines = append(lines, fmt.Sprintf("%s %s (%s)", cursorMark(i == m.pickerCursor), item.Title, item.Vault))
	}
	return strings.Join(lines, "\n")
}

func (m Model) fieldPickerView() string {
	lines := []string{m.styles.dim.Render("Pick a field")}
	for i, field := range m.pickerFields {
		lines = append(lines, fmt.Sprintf("%s %s  %s", cursorMark(i == m.fieldCursor), field.Label, field.Reference))
	}
	return strings.Join(lines, "\n")
}

var _ = opcli.Item{}
var _ = mapping.Secret{}
