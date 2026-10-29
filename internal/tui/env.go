package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

// updateEnv handles keys in the Env view.
func (m Model) updateEnv(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	name, secret := m.envSelection()
	switch msg.String() {
	case "up", "k":
		m.envCursor = clampIndex(m.envCursor-1, len(m.envEntries()))
	case "down", "j":
		m.envCursor = clampIndex(m.envCursor+1, len(m.envEntries()))
	case "a":
		if name != "" {
			m.prompt, m.input, m.pending = promptEnvName, secret.Env, name
		}
	case "space", "d":
		if name == "" {
			return m, nil
		}
		if secret.Env == "" {
			m.notice = "no env var on " + name + "; press a to add"
			return m, nil
		}
		if msg.String() == "d" {
			m.prompt, m.pending = promptEnvRemove, name
			return m, nil
		}
		return m, m.toggleEnv(name, secret.Env, secret.IsEnabled())
	}
	return m, nil
}

// envEntries lists every vault entry and every mapped pass, sorted, so a
// local entry whose pass file is missing can still be fixed here.
func (m Model) envEntries() []string {
	names := slices.Clone(m.vaultNames)
	for _, secret := range m.mapping.Secrets {
		if !slices.Contains(names, secret.Pass) {
			names = append(names, secret.Pass)
		}
	}
	slices.Sort(names)
	return names
}

// envSelection returns the entry name under the Env cursor and its secret.
func (m Model) envSelection() (string, mapping.Secret) {
	entries := m.envEntries()
	if m.envCursor >= len(entries) {
		return "", mapping.Secret{}
	}
	secret, _ := m.mapping.Find(entries[m.envCursor])
	return entries[m.envCursor], secret
}

// toggleEnv turns the env variable off, or back on by dropping the flag.
func (m Model) toggleEnv(name, env string, enabled bool) tea.Cmd {
	note := "env " + env + " on"
	if enabled {
		note = "env " + env + " off"
	}
	return m.editSecretCmd(name, note, func(secret *mapping.Secret) {
		if secret.IsEnabled() {
			off := false
			secret.Enabled = &off
		} else {
			secret.Enabled = nil
		}
	})
}

// submitEnvName sets or renames the env variable of the prompt's entry.
func (m Model) submitEnvName() (tea.Model, tea.Cmd) {
	name, env := m.pending, strings.ToUpper(strings.TrimSpace(m.input))
	m.prompt, m.input, m.pending = promptNone, "", ""
	if env == "" {
		m.notice = "env unchanged"
		return m, nil
	}
	return m, m.editSecretCmd(name, "env "+env+" set on "+name, func(secret *mapping.Secret) {
		secret.Env = env
	})
}

// submitEnvRemove removes the env variable only when the typed name matches.
func (m Model) submitEnvRemove() (tea.Model, tea.Cmd) {
	name, typed := m.pending, strings.TrimSpace(m.input)
	m.prompt, m.input, m.pending = promptNone, "", ""
	secret, _ := m.mapping.Find(name)
	if typed != secret.Env {
		m.notice = "name did not match; env kept"
		return m, nil
	}
	return m, m.editSecretCmd(name, "env "+secret.Env+" removed", func(secret *mapping.Secret) {
		secret.Env, secret.Scope, secret.Commands, secret.Enabled = "", mapping.ScopeGlobal, nil, nil
	})
}

// editSecretCmd runs editSecret, then reports it so the model reloads.
func (m Model) editSecretCmd(pass, note string, change func(*mapping.Secret)) tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		if err := editSecret(deps, pass, change); err != nil {
			return errMsg{err}
		}
		return mutatedMsg{note}
	}
}

// editSecret applies change to the secret for pass as the file has it now,
// so edits made outside the TUI survive. It adds a local secret when none
// exists and removes a local one left without env.
func editSecret(deps Deps, pass string, change func(*mapping.Secret)) error {
	current, err := deps.Mapping()
	if err != nil {
		return err
	}
	secret, found := current.Find(pass)
	secret.Pass = pass
	change(&secret)
	switch {
	case !found:
		return mapping.AppendSecret(deps.MappingPath, secret)
	case secret.Env == "" && secret.Ref == "":
		return mapping.RemoveSecret(deps.MappingPath, pass)
	}
	return mapping.ReplaceSecret(deps.MappingPath, secret)
}

func (m Model) envPromptText() string {
	name := m.pending
	secret, _ := m.mapping.Find(name)
	if m.prompt == promptEnvName {
		return "Env name for " + name + "\n> " + m.input
	}
	return "Remove env " + secret.Env + " from " + name + " — type the env name to confirm\n> " + m.input
}

// envView renders one row per entry with its env variable settings.
func (m Model) envView() string {
	rows := [][]string{{" ", "ENTRY", "ENV", "SCOPE", "STATE", "SOURCE"}}
	for i, name := range m.envEntries() {
		secret, ok := m.mapping.Find(name)
		rows = append(rows, []string{cursorMark(i == m.envCursor), name, envName(secret),
			envScope(secret), m.envState(secret), envSource(secret, ok)})
	}
	table := alignColumns(rows)
	table[0] = m.styles.dim.Render(table[0])
	title := m.styles.title.Render("Env vars") + m.styles.dim.Render(" ─ "+m.deps.MappingPath)
	return strings.Join(append([]string{title, ""}, table...), "\n")
}

// alignColumns pads each cell to its column's widest visible value.
func alignColumns(rows [][]string) []string {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for column, cell := range row {
			widths[column] = max(widths[column], lipgloss.Width(cell))
		}
	}
	lines := make([]string, len(rows))
	for i, row := range rows {
		cells := make([]string, len(row))
		for column, cell := range row {
			cells[column] = cell + strings.Repeat(" ", widths[column]-lipgloss.Width(cell))
		}
		lines[i] = strings.TrimRight(strings.Join(cells, " "), " ")
	}
	return lines
}

func envName(secret mapping.Secret) string {
	if secret.Env == "" {
		return "—"
	}
	return secret.Env
}

func envScope(secret mapping.Secret) string {
	switch {
	case secret.Env == "":
		return "—"
	case secret.Scope == mapping.ScopeCommand:
		return "command: " + strings.Join(secret.Commands, ",")
	case secret.Scope == mapping.ScopeNone:
		return "none"
	}
	return "global"
}

func (m Model) envState(secret mapping.Secret) string {
	switch {
	case secret.Env == "":
		return "—"
	case secret.IsEnabled():
		return m.styles.ok.Render("●") + " on"
	}
	return m.styles.warn.Render("○") + " off"
}

func envSource(secret mapping.Secret, hasSecret bool) string {
	if hasSecret && secret.Ref != "" {
		return "1Password"
	}
	return "local"
}
