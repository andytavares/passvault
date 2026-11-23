package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
	"github.com/atavares-klaviyo/pass-vault/internal/store"
)

// planMsg previews a rollback of one revision.
type planMsg struct {
	revision string
	restored []string
	removed  []string
}

// updateHistory handles keys in the history view.
func (m Model) updateHistory(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.revCursor > 0 {
			m.revCursor--
		}
	case "down", "j":
		if m.revCursor < len(m.revisions)-1 {
			m.revCursor++
		}
	case "enter":
		return m.startRollback()
	case "f":
		m.historyEntry, m.revCursor = m.nextHistoryEntry(), 0
		return m, revisionsCmd(m.deps.Store, m.historyEntry)
	}
	return m, nil
}

// nextHistoryEntry steps from the whole store through each Vault row's
// name in sorted order, then back to the whole store.
func (m Model) nextHistoryEntry() string {
	names := make([]string, len(m.rows))
	for i, row := range m.rows {
		names[i] = row.Name
	}
	return nextFilter(names, m.historyEntry)
}

// startRollback previews the plan, then asks for typed confirmation.
func (m Model) startRollback() (tea.Model, tea.Cmd) {
	if m.revCursor >= len(m.revisions) {
		return m, nil
	}
	m.planTarget = m.revisions[m.revCursor].Short
	m.prompt, m.input = promptRollback, ""
	return m, planCmd(m.deps.Store, m.planTarget, m.historyEntry)
}

// applyPlan stores the rollback preview for the confirm prompt.
func (m Model) applyPlan(msg planMsg) (tea.Model, tea.Cmd) {
	m.planTarget, m.planRestored, m.planRemoved = msg.revision, msg.restored, msg.removed
	return m, nil
}

// historyView renders sync tags and commits, or one entry's changes.
func (m Model) historyView() string {
	if m.historyEntry != "" {
		return m.entryHistoryView()
	}
	lines := []string{m.styles.title.Render("History") + m.styles.dim.Render(" ─ sync tags and commits")}
	lines = append(lines, m.tagLines()...)
	lines = append(lines, "")
	lines = append(lines, m.revisionLines()...)
	return strings.Join(lines, "\n")
}

func (m Model) tagLines() []string {
	if len(m.syncTags) == 0 {
		return []string{m.styles.dim.Render("no sync tags yet")}
	}
	lines := []string{m.styles.dim.Render("Tags")}
	for _, tag := range m.syncTags {
		lines = append(lines, "  "+m.styles.ok.Render(tag))
	}
	return lines
}

func (m Model) revisionLines() []string {
	lines := []string{m.styles.dim.Render("Commits")}
	for i, revision := range m.revisions {
		lines = append(lines, m.revisionLine(i, revision))
	}
	return lines
}

func (m Model) revisionLine(index int, revision store.Revision) string {
	return fmt.Sprintf("%s %s %s %s",
		cursorMark(index == m.revCursor),
		m.styles.warn.Render(revision.Short),
		revision.Time.Format("01-02 15:04"),
		revision.Subject)
}

// rollbackPromptText describes the plan and the required confirmation word.
func (m Model) rollbackPromptText() string {
	lines := []string{fmt.Sprintf("Rollback to %s:", m.planTarget)}
	if m.historyEntry != "" {
		lines[0] = fmt.Sprintf("Rollback %s to %s:", m.historyEntry, m.planTarget)
	}
	for _, name := range m.planRestored {
		lines = append(lines, "  restore "+name)
	}
	for _, name := range m.planRemoved {
		lines = append(lines, "  remove  "+name)
	}
	lines = append(lines, `Type "rollback" and press enter to confirm`, "> "+m.input)
	return strings.Join(lines, "\n")
}

// entryHistoryView lists the commits that changed historyEntry beside the
// selected commit's detail.
func (m Model) entryHistoryView() string {
	header := m.styles.title.Render("History") +
		m.styles.dim.Render(fmt.Sprintf(" ─ %s · %d changes", m.historyEntry, len(m.revisions)))
	lines := []string{header}
	for i, revision := range m.revisions {
		lines = append(lines, fmt.Sprintf("%s %s %s  %s", cursorMark(i == m.revCursor),
			m.styles.warn.Render(revision.Short), revision.Time.Format("01-02 15:04"), revision.Change(m.historyEntry)))
	}
	detail := ""
	if m.revCursor < len(m.revisions) {
		detail = m.entryRevisionDetail(m.revisions[m.revCursor])
	}
	return m.splitPane(strings.Join(lines, "\n"), detail)
}

// entryRevisionDetail shows the change, commit and tags of one revision,
// and who made it when the audit log has its write event.
func (m Model) entryRevisionDetail(revision store.Revision) string {
	tags := "-"
	if len(revision.Tags) > 0 {
		tags = strings.Join(revision.Tags, ", ")
	}
	lines := []string{
		m.styles.title.Render(revision.Short + " · " + revision.Time.Format("2006-01-02 15:04")), "",
		"Change  " + revision.Change(m.historyEntry),
		"Commit  " + revision.Subject,
		"Tag     " + tags,
	}
	if event, ok := m.writeEvent(revision); ok {
		lines = append(lines, fmt.Sprintf("Who     %s (uid %d) on %s via %s", event.User, event.UID, event.Host, event.Via))
		lines = append(lines, callerLines(event.Callers)...)
	}
	return strings.Join(lines, "\n")
}

// writeEvent finds the audit event of the write that made revision; the
// store logs each write's commit as "commit <short hash>" in Detail.
func (m Model) writeEvent(revision store.Revision) (audit.Event, bool) {
	for _, event := range m.auditEvents {
		if event.Entry == m.historyEntry && event.Action == audit.ActionWrite &&
			strings.Contains(event.Detail, "commit "+revision.Short) {
			return event, true
		}
	}
	return audit.Event{}, false
}
