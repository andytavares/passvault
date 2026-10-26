package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

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
	}
	return m, nil
}

// startRollback previews the plan, then asks for typed confirmation.
func (m Model) startRollback() (tea.Model, tea.Cmd) {
	if m.revCursor >= len(m.revisions) {
		return m, nil
	}
	m.planTarget = m.revisions[m.revCursor].Short
	m.prompt, m.input = promptRollback, ""
	return m, planCmd(m.deps.Store, m.planTarget)
}

// applyPlan stores the rollback preview for the confirm prompt.
func (m Model) applyPlan(msg planMsg) (tea.Model, tea.Cmd) {
	m.planTarget, m.planRestored, m.planRemoved = msg.revision, msg.restored, msg.removed
	return m, nil
}

// historyView renders sync tags and commits.
func (m Model) historyView() string {
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
	for _, name := range m.planRestored {
		lines = append(lines, "  restore "+name)
	}
	for _, name := range m.planRemoved {
		lines = append(lines, "  remove  "+name)
	}
	lines = append(lines, `Type "rollback" and press enter to confirm`, "> "+m.input)
	return strings.Join(lines, "\n")
}
