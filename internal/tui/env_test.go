package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

var spaceKey = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}

// envModel opens the Env tab over alpha, beta and zeta (unmapped).
func envModel(t *testing.T, f *fixture) Model {
	t.Helper()
	for _, name := range []string{"alpha", "beta", "zeta"} {
		f.env.Insert(t, name, name+"-value")
	}
	m := f.loadModel(t)
	m.active = viewEnv
	return m
}

func findSecret(t *testing.T, f *fixture, pass string) (mapping.Secret, bool) {
	t.Helper()
	mapped, err := f.deps.Mapping()
	if err != nil {
		t.Fatal(err)
	}
	return mapped.Find(pass)
}

// runKey presses a key and applies the resulting command message.
func runKey(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		m = update(t, m, cmd())
	}
	return update(t, m, loadCmd(m.deps)())
}

// typeAndReload submits the prompt text, then reloads the mapping.
func typeAndReload(t *testing.T, m Model, text string) Model {
	t.Helper()
	m = typeKeys(t, m, text)
	return update(t, m, loadCmd(m.deps)())
}

func clearInput(m Model) Model {
	m.input = ""
	return m
}

func TestEnvAddOnUnmappedEntryAppendsLocalTable(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = update(t, update(t, m, press("j")), press("j"))
	m = update(t, m, press("a"))
	if m.prompt != promptEnvName {
		t.Fatalf("prompt = %v, want promptEnvName", m.prompt)
	}
	m = typeKeys(t, m, "zeta_key")
	secret, ok := findSecret(t, f, "zeta")
	if !ok || secret.Env != "ZETA_KEY" || secret.Ref != "" {
		t.Fatalf("secret = %+v, ok=%v; want local ZETA_KEY", secret, ok)
	}
	if m.notice != "env ZETA_KEY set on zeta" {
		t.Errorf("notice = %q", m.notice)
	}
}

func TestEnvAddOnAlphaRenamesAndKeepsRef(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = update(t, m, press("a"))
	if m.input != "ALPHA_KEY" {
		t.Fatalf("input = %q, want prefilled ALPHA_KEY", m.input)
	}
	m = clearInput(m)
	typeKeys(t, m, "RENAMED_KEY")
	secret, _ := findSecret(t, f, "alpha")
	if secret.Env != "RENAMED_KEY" || secret.Ref != "op://Private/Alpha/credential" {
		t.Fatalf("secret = %+v", secret)
	}
}

func TestEnvAddEmptyLeavesEnvUnchanged(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = update(t, m, press("a"))
	m = typeKeys(t, clearInput(m), "")
	if m.notice != "env unchanged" {
		t.Errorf("notice = %q", m.notice)
	}
	if secret, _ := findSecret(t, f, "alpha"); secret.Env != "ALPHA_KEY" {
		t.Errorf("env = %q, want ALPHA_KEY", secret.Env)
	}
}

func TestEnvSpaceTogglesEnabled(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = runKey(t, m, spaceKey)
	if secret, _ := findSecret(t, f, "alpha"); secret.IsEnabled() {
		t.Fatal("first space must turn alpha off")
	}
	if m.notice != "env ALPHA_KEY off" {
		t.Errorf("notice = %q", m.notice)
	}
	runKey(t, m, spaceKey)
	if secret, _ := findSecret(t, f, "alpha"); !secret.IsEnabled() {
		t.Fatal("second space must turn alpha on")
	}
}

func TestEnvSpaceAndRemoveOnEntryWithoutEnvShowNotice(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{spaceKey, press("d")} {
		f := newFixture(t)
		m := envModel(t, f)
		m = update(t, m, press("j"))
		before, err := os.ReadFile(f.deps.MappingPath)
		if err != nil {
			t.Fatal(err)
		}
		m = runKey(t, m, key)
		if m.notice != "no env var on beta; press a to add" {
			t.Errorf("notice = %q", m.notice)
		}
		if m.prompt != promptNone {
			t.Errorf("prompt = %v, want none", m.prompt)
		}
		after, _ := os.ReadFile(f.deps.MappingPath)
		if string(before) != string(after) {
			t.Errorf("mapping file changed:\n%s", after)
		}
	}
}

func TestEnvRemoveWrongNameKeepsEnv(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = update(t, m, press("d"))
	if m.prompt != promptEnvRemove {
		t.Fatalf("prompt = %v, want promptEnvRemove", m.prompt)
	}
	m = typeKeys(t, m, "WRONG")
	if m.notice != "name did not match; env kept" {
		t.Errorf("notice = %q", m.notice)
	}
	if secret, _ := findSecret(t, f, "alpha"); secret.Env != "ALPHA_KEY" {
		t.Errorf("env = %q, want kept", secret.Env)
	}
}

func TestEnvRemoveClearsEnvButKeepsRefTable(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = update(t, m, press("d"))
	m = typeKeys(t, m, "ALPHA_KEY")
	secret, ok := findSecret(t, f, "alpha")
	if !ok || secret.Env != "" || secret.Ref != "op://Private/Alpha/credential" {
		t.Fatalf("secret = %+v, ok=%v", secret, ok)
	}
}

func TestEnvRemoveOnLocalEntryRemovesTable(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = update(t, update(t, m, press("j")), press("j"))
	m = typeAndReload(t, update(t, m, press("a")), "ZETA_KEY")
	m = typeKeys(t, update(t, m, press("d")), "ZETA_KEY")
	if _, ok := findSecret(t, f, "zeta"); ok {
		t.Fatal("local table must be removed")
	}
	if m.notice != "env ZETA_KEY removed" {
		t.Errorf("notice = %q", m.notice)
	}
}

func TestEnvTabRendersRowsAndFooter(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = runKey(t, m, spaceKey)
	view := viewText(m)
	for _, want := range []string{"Env vars", "ENTRY", "STATE", "SOURCE", "alpha", "ALPHA_KEY", "global",
		"1Password", "local", "—", "add/rename env", "on/off", "remove env"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}
