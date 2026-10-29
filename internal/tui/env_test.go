package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
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

func TestEnvCursorStaysInRangeWhenRowsShrink(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m.envCursor = 2
	m = update(t, m, loadedMsg{vaultNames: []string{"alpha"}, rows: m.rows[:1]})
	if m.envCursor != 0 {
		t.Fatalf("envCursor = %d, want 0 after rows shrink to 1", m.envCursor)
	}
}

func TestEnvListsLocalEntryWithoutPassFileAndHidesMissingOrphans(t *testing.T) {
	f := newFixture(t)
	writeMapping(t, f.deps.MappingPath, testMapping+"\n[[secret]]\npass = \"notes/wfi\"\nenv = \"WIFI\"\n")
	f.state.Entries["gone"] = syncer.EntryState{Tag: "sync-1"}
	m := envModel(t, f)
	text := viewText(m)
	if !strings.Contains(text, "notes/wfi") {
		t.Fatalf("local entry without a pass file must be listed:\n%s", text)
	}
	if strings.Contains(text, "gone") {
		t.Fatalf("entry in neither vault nor mapping must not be listed:\n%s", text)
	}
}

func TestEnvPromptKeepsTargetWhenRowsChange(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = update(t, m, press("a"))
	m = clearInput(m)
	f.env.Insert(t, "aaa", "aaa-value")
	m = update(t, m, loadCmd(f.deps)())
	m = typeKeys(t, m, "ALPHA_RENAMED")
	if secret, _ := findSecret(t, f, "alpha"); secret.Env != "ALPHA_RENAMED" {
		t.Fatalf("alpha env = %q; the prompt changed target after a reload", secret.Env)
	}
	if _, ok := findSecret(t, f, "aaa"); ok {
		t.Fatal("aaa must not get the env name")
	}
}

func TestEnvToggleKeepsEditsMadeOutsideTheTUI(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	edited := strings.Replace(testMapping, "env = \"ALPHA_KEY\"\n", "env = \"ALPHA_KEY\"\nscope = \"none\"\n", 1)
	writeMapping(t, f.deps.MappingPath, edited)
	runKey(t, m, spaceKey)
	secret, _ := findSecret(t, f, "alpha")
	if secret.Scope != mapping.ScopeNone || secret.IsEnabled() {
		t.Fatalf("alpha = %+v; want scope none kept and enabled false", secret)
	}
}

func TestEnvColumnsAlignWithLongValues(t *testing.T) {
	f := newFixture(t)
	writeMapping(t, f.deps.MappingPath, testMapping+"\n[[secret]]\npass = \"terminator-app-specific-password\"\nenv = \"APPLE_APP_SPECIFIC_PASSWORD\"\n")
	lines := strings.Split(ansi.Strip(viewText(envModel(t, f))), "\n")
	header, row := "", ""
	for _, line := range lines {
		if strings.Contains(line, "ENTRY") {
			header = line
		}
		if strings.Contains(line, "terminator-app-specific-password") {
			row = line
		}
	}
	for _, column := range []struct{ title, value string }{{"ENV", "APPLE_APP"}, {"SCOPE", "global"}, {"SOURCE", "local"}} {
		got := ansi.StringWidth(row[:strings.Index(row, column.value)])
		if want := strings.Index(header, column.title); got != want {
			t.Errorf("%s starts at %d, header at %d:\n%s\n%s", column.value, got, want, header, row)
		}
	}
}

func TestEnvNewCreatesEntryAndLocalEnv(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = update(t, m, press("n"))
	m = typeKeys(t, m, "work/github")
	m = typeKeys(t, m, "github_token")
	if m.prompt != promptNewValue {
		t.Fatalf("prompt = %v, want promptNewValue", m.prompt)
	}
	m = typeKeys(t, m, "ghp-secret")
	if got := f.env.Show(t, "work/github"); got != "ghp-secret" {
		t.Fatalf("pass value = %q", got)
	}
	secret, ok := findSecret(t, f, "work/github")
	if !ok || secret.Env != "GITHUB_TOKEN" || secret.Ref != "" {
		t.Fatalf("secret = %+v, ok=%v; want local GITHUB_TOKEN", secret, ok)
	}
	if m.notice != "added work/github with env GITHUB_TOKEN" {
		t.Errorf("notice = %q", m.notice)
	}
}

func TestEnvNewRefusesExistingEntry(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = typeKeys(t, update(t, m, press("n")), "alpha")
	if m.prompt != promptNone || m.notice != "alpha exists; select it and press a" {
		t.Fatalf("prompt = %v, notice = %q", m.prompt, m.notice)
	}
	if got := f.env.Show(t, "alpha"); got != "alpha-value" {
		t.Fatalf("alpha value = %q, want unchanged", got)
	}
}

func TestEnvNewWithInvalidEnvCreatesNothing(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = typeKeys(t, update(t, m, press("n")), "work/bad")
	m = typeKeys(t, typeKeys(t, m, "bad-name"), "value")
	if m.err == nil {
		t.Fatal("want an error for env bad-name")
	}
	if _, ok := findSecret(t, f, "work/bad"); ok {
		t.Fatal("mapping must not change")
	}
	if f.deps.Store.Exists("work/bad") {
		t.Fatal("pass entry must not be created")
	}
}
