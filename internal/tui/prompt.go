package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

// updatePrompt routes keys while a modal input is open.
func (m Model) updatePrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.prompt, m.input, m.newEnv = promptNone, "", ""
		return m, nil
	case "enter":
		return m.submitPrompt()
	case "backspace":
		if m.input != "" {
			m.input = m.input[:len(m.input)-1]
		}
		return m, nil
	}
	if text := msg.Key().Text; text != "" {
		m.input += text
	}
	return m, nil
}

// submitPrompt validates the input and runs the prompt's action.
func (m Model) submitPrompt() (tea.Model, tea.Cmd) {
	switch m.prompt {
	case promptNewName:
		return m.submitNewName()
	case promptNewEnv:
		return m.submitNewEnv()
	case promptNewValue:
		return m.submitNewValue()
	case promptDelete:
		return m.submitDelete()
	case promptRollback:
		return m.submitRollback()
	case promptAddName:
		return m.nextPrompt(promptAddEnv)
	case promptAddEnv:
		return m.submitAddSecret()
	case promptEnvName:
		return m.submitEnvName()
	case promptEnvRemove:
		return m.submitEnvRemove()
	}
	return m, nil
}

// nextPrompt keeps the typed name in pending and opens the next prompt;
// an empty name keeps the current prompt open.
func (m Model) nextPrompt(next promptKind) (tea.Model, tea.Cmd) {
	if m.input == "" {
		return m, nil
	}
	m.pending, m.input, m.prompt = m.input, "", next
	return m, nil
}

// submitNewName opens the value prompt; the Env view asks for the env first.
func (m Model) submitNewName() (tea.Model, tea.Cmd) {
	if m.active == viewEnv {
		return m.submitEnvNewName()
	}
	return m.nextPrompt(promptNewValue)
}

// submitNewValue stores the new entry, with its env name when one was given.
func (m Model) submitNewValue() (tea.Model, tea.Cmd) {
	name, value, env := m.pending, m.input, m.newEnv
	m.prompt, m.input, m.pending, m.newEnv = promptNone, "", "", ""
	if env != "" {
		return m, m.insertWithEnvCmd(name, env, value)
	}
	return m, insertCmd(m.deps.Store, name, value)
}

// submitDelete removes the entry only when the typed name matches exactly.
func (m Model) submitDelete() (tea.Model, tea.Cmd) {
	row, _ := m.selectedRow()
	typed := m.input
	m.prompt, m.input = promptNone, ""
	if row.Name == "" {
		return m, nil
	}
	if row.Name != strings.TrimSpace(typed) {
		m.notice = "name did not match; entry kept"
		return m, nil
	}
	return m, removeCmd(m.deps, row.Name)
}

// submitRollback runs the rollback only for the exact word "rollback".
func (m Model) submitRollback() (tea.Model, tea.Cmd) {
	target := m.planTarget
	typed := m.input
	m.prompt, m.input = promptNone, ""
	if typed != "rollback" {
		m.notice = "rollback cancelled"
		return m, nil
	}
	return m, rollbackCmd(m.deps.Store, target)
}

// submitAddSecret appends the picked field to the mapping file.
func (m Model) submitAddSecret() (tea.Model, tea.Cmd) {
	secret := mapping.Secret{
		Pass: m.pending,
		Ref:  m.pickerRef,
		Env:  strings.ToUpper(m.input),
	}
	m.prompt, m.input, m.pending = promptNone, "", ""
	m.pickerItems, m.pickerFields = nil, nil
	return m, appendCmd(m.deps.MappingPath, secret)
}

// promptView renders the active modal input.
func (m Model) promptView() string {
	switch m.prompt {
	case promptNewName:
		return "New entry name\n> " + m.input
	case promptNewValue:
		return "Value for " + m.pending + " (hidden)\n> " + mask(m.input)
	case promptDelete:
		row, _ := m.selectedRow()
		return "Delete " + row.Name + " — type the name to confirm\n> " + m.input
	case promptRollback:
		return m.rollbackPromptText()
	case promptAddName:
		return "pass name\n> " + m.input
	case promptAddEnv:
		return "env name (empty for none)\n> " + m.input
	case promptNewEnv:
		return "Env name for " + m.pending + "\n> " + m.input
	case promptEnvName, promptEnvRemove:
		return m.envPromptText()
	}
	return ""
}

// mask replaces every character with a bullet.
func mask(value string) string {
	return strings.Repeat("•", len(value))
}
