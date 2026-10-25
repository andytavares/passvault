package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

// revealDuration is how long a revealed value stays visible.
const revealDuration = 10 * time.Second

// glyphs mark each StatusRow kind in the entries list.
var glyphs = map[syncer.StatusKind]string{
	syncer.KindMapped:        "●",
	syncer.KindMappedMissing: "◐",
	syncer.KindUnmanaged:     "○",
	syncer.KindOrphan:        "✕",
}

// applyLoaded stores the fresh rows and keeps the cursor in range.
func (m Model) applyLoaded(msg loadedMsg) (tea.Model, tea.Cmd) {
	m.rows, m.state, m.opOK, m.mapErrors = msg.rows, msg.state, msg.opOK, msg.mapErr
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
	m.err = nil
	if row, ok := m.selectedRow(); ok {
		return m, historyCmd(m.deps.Store, row.Name)
	}
	m.history = nil
	return m, nil
}

// applyReveal shows the decrypted value until the deadline.
func (m Model) applyReveal(msg revealMsg) (tea.Model, tea.Cmd) {
	m.reveal = &revealState{name: msg.name, value: msg.value, deadline: m.deps.Now().Add(revealDuration)}
	return m, nil
}

// applyTick re-masks the value once the reveal deadline passes.
func (m Model) applyTick(msg tickMsg) (tea.Model, tea.Cmd) {
	if m.reveal != nil && !msg.at.Before(m.reveal.deadline) {
		m.clearReveal()
	}
	return m, nil
}

// updateVault handles keys in the vault view.
func (m Model) updateVault(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		return m.moveCursor(-1)
	case "down", "j":
		return m.moveCursor(1)
	case "r":
		return m.revealSelected()
	case "y":
		return m.copySelected()
	case "n":
		m.prompt, m.input = promptNewName, ""
		return m, nil
	case "d":
		return m.startDelete()
	case "s":
		return m.startSync()
	case "c":
		return m, checkCmd(m.deps)
	}
	return m, nil
}

// moveCursor moves the selection; it never decrypts.
func (m Model) moveCursor(delta int) (tea.Model, tea.Cmd) {
	next := m.cursor + delta
	if next < 0 || next >= len(m.rows) {
		return m, nil
	}
	m.cursor = next
	m.clearReveal()
	return m, historyCmd(m.deps.Store, m.rows[m.cursor].Name)
}

func (m Model) revealSelected() (tea.Model, tea.Cmd) {
	row, ok := m.selectedRow()
	if !ok {
		return m, nil
	}
	return m, revealCmd(m.deps.Store, row.Name)
}

func (m Model) copySelected() (tea.Model, tea.Cmd) {
	row, ok := m.selectedRow()
	if !ok {
		return m, nil
	}
	m.notice = "copied " + row.Name + " (clears in 45 s)"
	return m, copyCmd(m.deps.Store, row.Name)
}

func (m Model) startDelete() (tea.Model, tea.Cmd) {
	if _, ok := m.selectedRow(); !ok {
		return m, nil
	}
	m.prompt, m.input = promptDelete, ""
	return m, nil
}

func (m Model) startSync() (tea.Model, tea.Cmd) {
	if m.syncing {
		return m, nil
	}
	m.active, m.syncing, m.syncLines = viewSync, true, nil
	return m, syncCmd(&m)
}

// vaultView renders the entries list beside the detail pane.
func (m Model) vaultView() string {
	list := m.entriesList()
	detail := m.detailPane()
	return lipglossJoin(list, detail)
}

// entriesList renders the left column of entry rows.
func (m Model) entriesList() string {
	var lines []string
	lines = append(lines, m.styles.dim.Render("Entries"))
	for i, row := range m.rows {
		lines = append(lines, m.entryLine(i, row))
	}
	return strings.Join(lines, "\n")
}

func (m Model) entryLine(index int, row syncer.StatusRow) string {
	label := fmt.Sprintf("%s %-28s %s", cursorMark(index == m.cursor), row.Name, m.glyph(row))
	return label + " " + m.statusLabel(row.Kind, age(row.SyncedAt, m.deps.Now()))
}

// glyph marks the row's kind; a synced entry counts as synced only when the
// state has a sync time for it.
func (m Model) glyph(row syncer.StatusRow) string {
	if row.Kind == syncer.KindMapped && row.SyncedAt.IsZero() {
		return glyphs[syncer.KindMappedMissing]
	}
	return glyphs[row.Kind]
}

func cursorMark(selected bool) string {
	if selected {
		return "▸"
	}
	return " "
}

func (m Model) statusLabel(kind syncer.StatusKind, ageText string) string {
	switch kind {
	case syncer.KindMapped:
		return m.styles.ok.Render("synced " + ageText)
	case syncer.KindMappedMissing:
		return m.styles.warn.Render("never")
	case syncer.KindOrphan:
		return m.styles.bad.Render("orphan")
	default:
		return m.styles.dim.Render("unmanaged")
	}
}

// detailPane renders ref, env, scope, masked value and history.
func (m Model) detailPane() string {
	row, ok := m.selectedRow()
	if !ok {
		return m.styles.dim.Render("no entries")
	}
	lines := []string{m.styles.title.Render(row.Name), ""}
	lines = append(lines, m.detailFields(row)...)
	lines = append(lines, "", m.styles.dim.Render("History"))
	lines = append(lines, m.historyLines()...)
	return strings.Join(lines, "\n")
}

func (m Model) detailFields(row syncer.StatusRow) []string {
	fields := []string{}
	if secret, ok := m.mapping.Find(row.Name); ok {
		fields = append(fields, "1Password  "+secret.Ref)
	}
	fields = append(fields, "Env        "+envOrDash(row.Env)+"  "+string(row.Scope))
	fields = append(fields, "Value      "+m.valueText(row))
	if !row.SyncedAt.IsZero() {
		fields = append(fields, "Last sync  "+row.SyncedAt.Format("2006-01-02 15:04")+"  "+row.Tag)
	}
	return fields
}

// valueText shows the revealed value or the masked placeholder. The length
// appears only after a reveal, never from state.
func (m Model) valueText(row syncer.StatusRow) string {
	if m.reveal != nil && m.reveal.name == row.Name {
		return m.reveal.value
	}
	return strings.Repeat("•", 16) + "  press r to reveal"
}

func (m Model) historyLines() []string {
	var lines []string
	for _, revision := range m.history {
		lines = append(lines, fmt.Sprintf("%s %s %s",
			m.styles.warn.Render(revision.Short),
			revision.Time.Format("01-02 15:04"),
			revision.Subject))
	}
	return lines
}

// envOrDash renders an empty env name as "-", like internal/cli's dash.
func envOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

var _ = mapping.Secret{}
