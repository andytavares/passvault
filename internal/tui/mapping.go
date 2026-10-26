package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/opcli"
)

// updateMapping handles keys in the mapping view.
func (m Model) updateMapping(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		m.picker, m.pickerCursor, m.pickerQuery, m.pickerVault = pickerItems, 0, "", ""
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
// runs before the global keys so letters, q and ←/→ reach the search and
// vault filter. It reports handled=false when no picker is active.
func (m Model) updatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if len(m.pickerItems) == 0 || msg.String() == "ctrl+c" {
		return m, nil, false
	}
	switch msg.String() {
	case "up":
		m.movePicker(-1)
	case "down":
		m.movePicker(1)
	case "enter":
		return m.pickCurrent()
	case "esc":
		m.pickerItems, m.pickerFields = nil, nil
	default:
		m.editPickerFilter(msg)
	}
	return m, nil, true
}

// editPickerFilter changes the item search or vault filter and resets the
// cursor to the first match. Field picking has no filter.
func (m *Model) editPickerFilter(msg tea.KeyPressMsg) {
	if m.picker != pickerItems {
		return
	}
	switch msg.String() {
	case "left":
		m.pickerVault = cycleVault(vaultNames(m.pickerItems), m.pickerVault, -1)
	case "right":
		m.pickerVault = cycleVault(vaultNames(m.pickerItems), m.pickerVault, 1)
	case "backspace":
		query := []rune(m.pickerQuery)
		m.pickerQuery = string(query[:max(0, len(query)-1)])
	default:
		m.pickerQuery += msg.Key().Text
	}
	m.pickerCursor = 0
}

// filteredItems are the picker items in the chosen vault whose title
// contains the search, ignoring case.
func (m Model) filteredItems() []opcli.Item {
	query := strings.ToLower(m.pickerQuery)
	var items []opcli.Item
	for _, item := range m.pickerItems {
		inVault := m.pickerVault == "" || item.Vault == m.pickerVault
		if inVault && strings.Contains(strings.ToLower(item.Title), query) {
			items = append(items, item)
		}
	}
	return items
}

// vaultNames returns the distinct vaults of items, sorted.
func vaultNames(items []opcli.Item) []string {
	var names []string
	for _, item := range items {
		names = append(names, item.Vault)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// cycleVault steps through "" (all vaults) and then each vault, wrapping.
func cycleVault(vaults []string, current string, delta int) string {
	options := append([]string{""}, vaults...)
	index := max(0, slices.Index(options, current))
	return options[(index+delta+len(options))%len(options)]
}

func (m *Model) movePicker(delta int) {
	if m.picker == pickerItems {
		m.pickerCursor = clampIndex(m.pickerCursor+delta, len(m.filteredItems()))
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
	items := m.filteredItems()
	if len(items) == 0 {
		return m, nil, true
	}
	if m.picker == pickerItems {
		m.picker = pickerFields
		return m, fieldsCmd(m.deps.Items, items[m.pickerCursor].ID), true
	}
	if len(m.pickerFields) == 0 {
		return m, nil, true
	}
	m.pickerRef = m.pickerFields[m.fieldCursor].Reference
	m.prompt, m.input = promptAddName, slugify(items[m.pickerCursor].Title)
	return m, nil, true
}

// mappingView renders mapping entries, or the picker in their place when
// open so it gets the whole body to scroll in.
func (m Model) mappingView() string {
	lines := []string{m.styles.title.Render("Mapping") + m.styles.dim.Render(" ─ "+m.deps.MappingPath)}
	if len(m.pickerItems) > 0 {
		return strings.Join(append(lines, m.pickerView()), "\n")
	}
	if m.mapErrors != "" {
		lines = append(lines, m.styles.bad.Render(m.mapErrors))
	}
	lines = append(lines, m.secretLines()...)
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
	vault := m.pickerVault
	if vault == "" {
		vault = "all"
	}
	lines := []string{
		m.styles.dim.Render("Add from 1Password · pick an item"),
		fmt.Sprintf("vault: %s   search: %s_", vault, m.pickerQuery),
	}
	items := m.filteredItems()
	if len(items) == 0 {
		lines = append(lines, m.styles.dim.Render("no matching items"))
	}
	// The mapping title line sits above these lines.
	start, end := visibleRange(m.pickerCursor, len(items), m.bodyHeight()-len(lines)-1)
	for i := start; i < end; i++ {
		lines = append(lines, fmt.Sprintf("%s %s (%s)", cursorMark(i == m.pickerCursor), items[i].Title, items[i].Vault))
	}
	return strings.Join(lines, "\n")
}

// visibleRange returns the rows of a list of length that fit in height
// and keep cursor on screen.
func visibleRange(cursor, length, height int) (start, end int) {
	height = max(1, height)
	start = max(0, cursor-height+1)
	return start, min(length, start+height)
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
