package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
)

const commandScopeMapping = `version = 1

[[secret]]
pass = "alpha"
ref = "op://Private/Alpha/credential"
env = "ALPHA_KEY"

[[secret]]
pass = "beta"
ref = "op://Private/Beta/credential"
env = "BETA_KEY"
scope = "command"
commands = ["gh"]
`

// holdersModel loads alpha (global) and beta (command scope) with a fake
// Holders that reports two processes for ALPHA_KEY.
func holdersModel(t *testing.T) Model {
	t.Helper()
	f := newFixture(t)
	writeMapping(t, f.deps.MappingPath, commandScopeMapping)
	f.env.Insert(t, "alpha", "alpha-value")
	f.env.Insert(t, "beta", "beta-value")
	f.deps.Holders = func(envName string) ([]audit.Process, error) {
		if envName != "ALPHA_KEY" {
			return nil, nil
		}
		return []audit.Process{
			{PID: 4120, Comm: "-zsh"},
			{PID: 5310, Comm: "node", Argv: []string{"/usr/local/bin/node", "server.js"}},
		}, nil
	}
	return f.loadModel(t)
}

func TestHoldersKeyShowsProcessesStartedWithGlobalEnv(t *testing.T) {
	m := holdersModel(t)
	next, cmd := m.Update(press("p"))
	m = update(t, next.(Model), cmd())
	view := ansi.Strip(viewText(m))
	for _, want := range []string{
		"│ Held by    2 processes started with ALPHA_KEY",
		"│   4120  -zsh",
		"│   5310  node server.js",
		"│ macOS hides Apple's own programs",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if view = ansi.Strip(viewText(update(t, m, press("p")))); strings.Contains(view, "Held by") {
		t.Errorf("second p should hide the holders:\n%s", view)
	}
}

func TestHoldersKeyOnCommandScopeEntrySetsNotice(t *testing.T) {
	m := update(t, holdersModel(t), press("j"))
	next, cmd := m.Update(press("p"))
	if cmd != nil || next.(Model).notice != "p works on global entries with an env variable" {
		t.Fatalf("p on beta: cmd=%v notice=%q", cmd != nil, next.(Model).notice)
	}
}

func TestVaultFooterListsHistoryAndHoldersKeys(t *testing.T) {
	m := holdersModel(t)
	footer := ansi.Strip(m.footer())
	if !strings.Contains(footer, "enter show/hide • c copy • h history • p holders • s sync") || strings.Contains(footer, "n new") {
		t.Errorf("short footer = %q", footer)
	}
	m = update(t, m, press("?"))
	full := ansi.Strip(m.footer())
	if newKey, refresh := strings.Fields(lineWith(full, "new")), strings.Fields(lineWith(full, "refresh")); !slices.Equal(newKey, []string{"n", "new"}) || !slices.Equal(refresh, []string{"r", "refresh"}) {
		t.Errorf("full help lacks n new and r refresh: %q", full)
	}
}

func TestHoldersListLongerThanPaneEndsWithCount(t *testing.T) {
	m := holdersModel(t)
	many := make([]audit.Process, 40)
	for i := range many {
		many[i] = audit.Process{PID: 6000 + i, Comm: "node"}
	}
	m.deps.Holders = func(string) ([]audit.Process, error) { return many, nil }
	next, cmd := m.Update(press("p"))
	view := ansi.Strip(viewText(update(t, next.(Model), cmd())))
	for _, want := range []string{"│ Held by    40 processes started with ALPHA_KEY", "more not shown", "│ macOS hides Apple's own programs"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}
