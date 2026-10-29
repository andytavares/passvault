package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// minWidth is the narrowest terminal the TUI renders for.
const minWidth = 60

// styles holds the palette, derived from the terminal background.
type styles struct {
	title     lipgloss.Style
	dim       lipgloss.Style
	ok        lipgloss.Style
	warn      lipgloss.Style
	bad       lipgloss.Style
	tab       lipgloss.Style
	activeTab lipgloss.Style
	box       lipgloss.Style
}

// newStyles picks colors that work on light and dark backgrounds.
func newStyles(isDark bool) styles {
	lightDark := lipgloss.LightDark(isDark)
	return styles{
		title:     lipgloss.NewStyle().Bold(true),
		dim:       lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("#6b6b6b"), lipgloss.Color("#9c9c9c"))),
		ok:        lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("#1a7f37"), lipgloss.Color("#3fb950"))),
		warn:      lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("#9a6700"), lipgloss.Color("#d29922"))),
		bad:       lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("#cf222e"), lipgloss.Color("#f85149"))),
		tab:       lipgloss.NewStyle().Padding(0, 1).Foreground(lightDark(lipgloss.Color("#6b6b6b"), lipgloss.Color("#9c9c9c"))),
		activeTab: lipgloss.NewStyle().Padding(0, 1).Bold(true).Reverse(true),
		box:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
	}
}

// View renders the active view full screen, or the too-narrow notice.
func (m Model) View() tea.View {
	content := m.render(m.body())
	if m.width > 0 && m.width < minWidth {
		content = "terminal too narrow (need 60 columns)"
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

// body picks the active view's content.
func (m Model) body() string {
	if m.prompt != promptNone {
		return lipgloss.Place(m.width, m.bodyHeight(), lipgloss.Center, lipgloss.Center, m.styles.box.Render(m.promptView()))
	}
	switch m.active {
	case viewVault:
		return m.vaultView()
	case viewSync:
		return m.syncView()
	case viewHistory:
		return m.historyView()
	case viewMapping:
		return m.mappingView()
	case viewEnv:
		return m.envView()
	}
	return ""
}
