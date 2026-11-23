package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

// entriesWidth fits the cursor, a 28-column name, glyph and status.
const entriesWidth = 50

// nameWidth is the widest entry name the list shows; longer names end in "…".
const nameWidth = 27

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
	m.mapping, m.rows, m.state, m.opOK, m.mapErrors = msg.mapping, msg.rows, msg.state, msg.opOK, msg.mapErr
	m.cursor = clampIndex(m.cursor, len(m.rows))
	m.vaultNames = msg.vaultNames
	m.envCursor = clampIndex(m.envCursor, len(m.envEntries()))
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
	case "enter":
		return m.toggleReveal()
	case "c":
		return m.copySelected()
	case "n":
		m.prompt, m.input = promptNewName, ""
		return m, nil
	case "d":
		return m.startDelete()
	case "h":
		return m.openEntryHistory()
	case "p":
		return m.toggleHolders()
	case "s":
		return m.startSync()
	case "r":
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
	m.holders = nil
	return m, historyCmd(m.deps.Store, m.rows[m.cursor].Name)
}

// toggleReveal decrypts the selected value, or hides it when shown.
func (m Model) toggleReveal() (tea.Model, tea.Cmd) {
	row, ok := m.selectedRow()
	if !ok {
		return m, nil
	}
	if m.reveal != nil && m.reveal.name == row.Name {
		m.clearReveal()
		return m, nil
	}
	return m, revealCmd(m.deps.Store, row.Name)
}

// openEntryHistory opens History filtered to the selected entry.
func (m Model) openEntryHistory() (tea.Model, tea.Cmd) {
	row, ok := m.selectedRow()
	if !ok {
		return m, nil
	}
	m.historyEntry, m.revCursor = row.Name, 0
	return m.switchView(viewHistory)
}

// toggleHolders lists the processes started with the selected global
// entry's env variable, or hides the list when shown.
func (m Model) toggleHolders() (tea.Model, tea.Cmd) {
	row, ok := m.selectedRow()
	if !ok {
		return m, nil
	}
	if m.holders != nil {
		m.holders = nil
		return m, nil
	}
	secret, found := m.mapping.Find(row.Name)
	if !found || secret.Scope != mapping.ScopeGlobal || secret.Env == "" || !secret.IsEnabled() {
		m.notice = "p works on global entries with an env variable"
		return m, nil
	}
	return m, holdersCmd(m.deps.Holders, row.Name, secret.Env)
}

// applyHolders stores the processes for the detail pane.
func (m Model) applyHolders(msg holdersMsg) (tea.Model, tea.Cmd) {
	m.holders = &msg
	return m, nil
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
	return m.splitPane(m.entriesList(), m.detailPane())
}

// splitPane renders list in the left column and detail right of a rule.
func (m Model) splitPane(list, detail string) string {
	left := lipgloss.NewStyle().Width(entriesWidth).Render(list)
	right := lipgloss.NewStyle().
		Height(m.bodyHeight()).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(m.styles.dim.GetForeground()).
		PaddingLeft(1).
		Render(detail)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
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
	auditMark := " "
	if m.mapping.IsAudited(row.Name) {
		auditMark = m.auditMark()
	}
	name := ansi.Truncate(row.Name, nameWidth, "…")
	label := fmt.Sprintf("%s %-*s %s %s", cursorMark(index == m.cursor), nameWidth, name, auditMark, m.glyph(row))
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

// auditMark flags an entry whose reveals and copies are logged.
func (m Model) auditMark() string {
	return m.styles.warn.Render("◉")
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
	lines = append(lines, m.holderLines(row.Name, m.bodyHeight()-len(lines))...)
	return strings.Join(lines, "\n")
}

// holderLines list the processes started with the entry's env variable,
// once p has loaded them for this entry, in at most room lines.
func (m Model) holderLines(name string, room int) []string {
	if m.holders == nil || m.holders.name != name {
		return nil
	}
	processes := m.holders.processes
	summary := fmt.Sprintf("Held by    %d processes started with %s", len(processes), m.holders.env)
	if len(processes) == 0 {
		summary = "Held by    no process started with " + m.holders.env
	}
	// blank, summary, the "more" line and the macOS note take four lines.
	shown := processes[:min(len(processes), max(0, room-4))]
	lines := append([]string{"", summary}, processLines(shown)...)
	if hidden := len(processes) - len(shown); hidden > 0 {
		lines = append(lines, m.styles.dim.Render(fmt.Sprintf("  %d more not shown", hidden)))
	}
	return append(lines, m.styles.dim.Render("macOS hides Apple's own programs"))
}

// processLines renders each process as its right-aligned pid and command.
func processLines(processes []audit.Process) []string {
	width := 0
	for _, process := range processes {
		width = max(width, len(fmt.Sprint(process.PID)))
	}
	lines := make([]string, len(processes))
	for i, process := range processes {
		lines[i] = fmt.Sprintf("  %*d  %s", width, process.PID, callerCommand(process))
	}
	return lines
}

func (m Model) detailFields(row syncer.StatusRow) []string {
	fields := []string{}
	if secret, ok := m.mapping.Find(row.Name); ok {
		fields = append(fields, "1Password  "+secret.Ref)
	}
	fields = append(fields, "Env        "+envOrDash(row.Env)+"  "+string(row.Scope))
	if m.mapping.IsAudited(row.Name) {
		fields = append(fields, "Audit      on · reveal and copy are logged")
	}
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
	return strings.Repeat("•", 16) + "  press enter to reveal"
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
