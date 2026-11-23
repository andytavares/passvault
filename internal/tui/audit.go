package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
)

const auditEmptyText = "No audit events. Mark a secret with A in Env, or audit = true in mapping.toml."

const auditHiddenText = "Only sync and compare rows here. Press a to show them."

// updateAudit handles keys in the Audit view. It never decrypts.
func (m Model) updateAudit(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.auditCursor = clampIndex(m.auditCursor-1, len(m.visibleAuditEvents()))
	case "down", "j":
		m.auditCursor = clampIndex(m.auditCursor+1, len(m.visibleAuditEvents()))
	case "f":
		m.auditFilter, m.auditCursor = m.nextAuditFilter(), 0
	case "a":
		m.auditShowAll, m.auditCursor = !m.auditShowAll, 0
	}
	return m, nil
}

// visibleAuditEvents are the loaded events that match the entry filter,
// without routine rows unless auditShowAll is set.
func (m Model) visibleAuditEvents() []audit.Event {
	var events []audit.Event
	for _, event := range m.scopedAuditEvents() {
		if m.auditShowAll || !isRoutine(event) {
			events = append(events, event)
		}
	}
	return events
}

// scopedAuditEvents are the loaded events that match the entry filter.
func (m Model) scopedAuditEvents() []audit.Event {
	if m.auditFilter == "" {
		return m.auditEvents
	}
	var events []audit.Event
	for _, event := range m.auditEvents {
		if event.Entry == m.auditFilter {
			events = append(events, event)
		}
	}
	return events
}

// isRoutine marks the compares and sync writes that every sync and check
// logs; they bury the reads a person makes.
func isRoutine(event audit.Event) bool {
	return event.Action == audit.ActionCompare || event.Via == "sync"
}

// nextAuditFilter steps from all entries through each logged entry in
// sorted order, then back to all.
func (m Model) nextAuditFilter() string {
	var entries []string
	for _, event := range m.auditEvents {
		if !slices.Contains(entries, event.Entry) {
			entries = append(entries, event.Entry)
		}
	}
	return nextFilter(entries, m.auditFilter)
}

// nextFilter steps from all ("") through names in sorted order, then back
// to all.
func nextFilter(names []string, current string) string {
	names = slices.Sorted(slices.Values(names))
	next := slices.Index(names, current) + 1
	if current == "" {
		next = 0
	}
	if next >= len(names) {
		return ""
	}
	return names[next]
}

// auditView renders the event list beside the selected event's detail.
func (m Model) auditView() string {
	events := m.visibleAuditEvents()
	scope := m.auditFilter
	if scope == "" {
		scope = "all entries"
	}
	header := m.styles.title.Render("Audit") + m.styles.dim.Render(" ─ "+scope+" · "+m.auditCount(len(events)))
	if len(events) == 0 {
		return header + "\n" + m.styles.dim.Render(m.auditEmptyHint())
	}
	return m.splitPane(header+"\n"+m.auditLines(events), m.auditDetail(events[m.auditCursor]))
}

// auditEmptyHint explains an empty list: no events at all, or only hidden ones.
func (m Model) auditEmptyHint() string {
	if len(m.scopedAuditEvents()) > 0 {
		return auditHiddenText
	}
	return auditEmptyText
}

// auditCount says how many events show and, when routine rows are hidden,
// how many of the entry's events are hidden.
func (m Model) auditCount(visible int) string {
	scoped := len(m.scopedAuditEvents())
	if visible == scoped {
		return fmt.Sprintf("%d events", visible)
	}
	return fmt.Sprintf("%d of %d events · %d hidden", visible, scoped, scoped-visible)
}

func (m Model) auditLines(events []audit.Event) string {
	width := 0
	for _, event := range events {
		width = max(width, len(event.Entry))
	}
	lines := make([]string, len(events))
	for i, event := range events {
		lines[i] = fmt.Sprintf("%s %s  %-*s  %-8s %s", cursorMark(i == m.auditCursor),
			event.Time.UTC().Format("01-02 15:04"), width, event.Entry, event.Action, event.Via)
	}
	return strings.Join(lines, "\n")
}

// auditDetail shows when, what, who and the caller chain of one event.
func (m Model) auditDetail(event audit.Event) string {
	lines := []string{
		m.styles.title.Render(event.Time.UTC().Format("2006-01-02 15:04:05 UTC")), "",
		"Entry   " + event.Entry,
		fmt.Sprintf("Action  %s  via %s", event.Action, event.Via),
		fmt.Sprintf("Who     %s (uid %d) on %s", event.User, event.UID, event.Host),
	}
	if event.Detail != "" {
		lines = append(lines, "Detail  "+event.Detail)
	}
	lines = append(lines, "")
	return strings.Join(append(lines, callerLines(event.Callers)...), "\n")
}

// callerLines lists each caller as its right-aligned pid and command line.
func callerLines(callers []audit.Process) []string {
	width := 0
	for _, caller := range callers {
		width = max(width, len(fmt.Sprint(caller.PID)))
	}
	lines := make([]string, len(callers))
	for i, caller := range callers {
		label := "        "
		if i == 0 {
			label = "What    "
		}
		lines[i] = fmt.Sprintf("%s%*d  %s", label, width, caller.PID, callerCommand(caller))
	}
	return lines
}

// callerCommand is argv with argv[0] shortened to its base name, or the
// process name when argv could not be read.
func callerCommand(caller audit.Process) string {
	if len(caller.Argv) == 0 {
		return caller.Comm
	}
	return strings.Join(append([]string{filepath.Base(caller.Argv[0])}, caller.Argv[1:]...), " ")
}
