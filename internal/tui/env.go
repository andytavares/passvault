package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

// updateEnv handles keys in the Env view.
func (m Model) updateEnv(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	name, secret, hasSecret := m.envSelection()
	switch msg.String() {
	case "up", "k":
		m.envCursor = clampIndex(m.envCursor-1, len(m.rows))
	case "down", "j":
		m.envCursor = clampIndex(m.envCursor+1, len(m.rows))
	case "a":
		if name != "" {
			m.prompt, m.input = promptEnvName, secret.Env
		}
	case "space", "d":
		if name == "" {
			return m, nil
		}
		if secret.Env == "" || !hasSecret {
			m.notice = "no env var on " + name + "; press a to add"
			return m, nil
		}
		if msg.String() == "d" {
			m.prompt = promptEnvRemove
			return m, nil
		}
		return m, m.toggleEnv(secret)
	}
	return m, nil
}

// envSelection returns the entry name under the Env cursor and its secret.
func (m Model) envSelection() (string, mapping.Secret, bool) {
	if m.envCursor >= len(m.rows) {
		return "", mapping.Secret{}, false
	}
	name := m.rows[m.envCursor].Name
	secret, ok := m.mapping.Find(name)
	return name, secret, ok
}

// toggleEnv turns the env variable off, or back on by dropping the flag.
func (m Model) toggleEnv(secret mapping.Secret) tea.Cmd {
	note := "env " + secret.Env + " on"
	if secret.IsEnabled() {
		off := false
		secret.Enabled, note = &off, "env "+secret.Env+" off"
	} else {
		secret.Enabled = nil
	}
	return replaceSecretCmd(m.deps.MappingPath, secret, note)
}

// submitEnvName sets or renames the env variable of the selected entry.
func (m Model) submitEnvName() (tea.Model, tea.Cmd) {
	name, secret, hasSecret := m.envSelection()
	env := strings.ToUpper(strings.TrimSpace(m.input))
	m.prompt, m.input = promptNone, ""
	if env == "" {
		m.notice = "env unchanged"
		return m, nil
	}
	secret.Pass, secret.Env = name, env
	note := "env " + env + " set on " + name
	if !hasSecret {
		return m, mappingWriteCmd(note, func() error { return mapping.AppendSecret(m.deps.MappingPath, secret) })
	}
	return m, replaceSecretCmd(m.deps.MappingPath, secret, note)
}

// submitEnvRemove removes the env variable only when the typed name matches.
func (m Model) submitEnvRemove() (tea.Model, tea.Cmd) {
	_, secret, _ := m.envSelection()
	typed := strings.TrimSpace(m.input)
	m.prompt, m.input = promptNone, ""
	if typed != secret.Env {
		m.notice = "name did not match; env kept"
		return m, nil
	}
	note := "env " + secret.Env + " removed"
	if secret.Ref == "" {
		return m, mappingWriteCmd(note, func() error { return mapping.RemoveSecret(m.deps.MappingPath, secret.Pass) })
	}
	secret.Env, secret.Scope, secret.Commands, secret.Enabled = "", mapping.ScopeGlobal, nil, nil
	return m, replaceSecretCmd(m.deps.MappingPath, secret, note)
}

func replaceSecretCmd(path string, secret mapping.Secret, note string) tea.Cmd {
	return mappingWriteCmd(note, func() error { return mapping.ReplaceSecret(path, secret) })
}

// mappingWriteCmd runs one mapping file write, then reports it so the
// model reloads.
func mappingWriteCmd(note string, write func() error) tea.Cmd {
	return func() tea.Msg {
		if err := write(); err != nil {
			return errMsg{err}
		}
		return mutatedMsg{note}
	}
}

func (m Model) envPromptText() string {
	name, secret, _ := m.envSelection()
	if m.prompt == promptEnvName {
		return "Env name for " + name + "\n> " + m.input
	}
	return "Remove env " + secret.Env + " from " + name + " — type the env name to confirm\n> " + m.input
}

// envView renders one row per entry with its env variable settings.
func (m Model) envView() string {
	lines := []string{
		m.styles.title.Render("Env vars") + m.styles.dim.Render(" ─ "+m.deps.MappingPath),
		"",
		m.styles.dim.Render(fmt.Sprintf("  %-24s %-20s %-14s %-7s %s", "ENTRY", "ENV", "SCOPE", "STATE", "SOURCE")),
	}
	for i, row := range m.rows {
		secret, ok := m.mapping.Find(row.Name)
		lines = append(lines, fmt.Sprintf("%s %-24s %-20s %-14s %s %s",
			cursorMark(i == m.envCursor), row.Name, envName(secret), envScope(secret),
			m.envState(secret), envSource(secret, ok)))
	}
	return strings.Join(lines, "\n")
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

// envState is padded to 7 columns; styling adds no visible width.
func (m Model) envState(secret mapping.Secret) string {
	switch {
	case secret.Env == "":
		return fmt.Sprintf("%-7s", "—")
	case secret.IsEnabled():
		return m.styles.ok.Render("●") + " on   "
	}
	return m.styles.warn.Render("○") + " off  "
}

func envSource(secret mapping.Secret, hasSecret bool) string {
	if hasSecret && secret.Ref != "" {
		return "1Password"
	}
	return "local"
}
