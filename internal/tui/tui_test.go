package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/opcli"
	"github.com/atavares-klaviyo/pass-vault/internal/store"
	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
	"github.com/atavares-klaviyo/pass-vault/internal/testenv"
)

const testMapping = `version = 1

[[secret]]
pass = "alpha"
ref = "op://Private/Alpha/credential"
env = "ALPHA_KEY"

[[secret]]
pass = "beta"
ref = "op://Private/Beta/credential"
`

// fixture builds a model over a real pass store, real git, a real opcli
// client and fake op fixtures.
type fixture struct {
	env   *testenv.Env
	deps  Deps
	state syncer.State
	now   time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	env := testenv.New(t).WithStore(t).WithGit(t)
	env.SetOpSignedIn(t, true)
	env.SetOp(t, "op://Private/Alpha/credential", "alpha-value")
	env.SetOp(t, "op://Private/Beta/credential", "beta-value")
	mappingPath := env.Paths.Mapping()
	writeMapping(t, mappingPath, testMapping)
	f := &fixture{env: env, now: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}
	f.state = syncer.State{Entries: map[string]syncer.EntryState{}}
	client := &opcli.Client{Runner: env.Runner()}
	f.deps = Deps{
		Mapping:     func() (mapping.Mapping, error) { return mapping.Load(mappingPath) },
		MappingPath: mappingPath,
		Store:       &store.Store{Runner: env.Runner(), Dir: env.Paths.StoreDir},
		Source:      client,
		Items:       client,
		State:       func() (syncer.State, error) { return f.state, nil },
		SaveState:   func(s syncer.State) error { f.state = s; return nil },
		Now:         func() time.Time { return f.now },
	}
	return f
}

func writeMapping(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// loadModel starts a model and applies the loadedMsg from loadCmd.
func (f *fixture) loadModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(f.deps)
	m.width, m.height = 100, 30
	next, _ := m.Update(loadCmd(f.deps)())
	model := next.(Model)
	if model.err != nil {
		t.Fatalf("load: %v", model.err)
	}
	return model
}

func press(name string) tea.KeyPressMsg {
	runes := []rune(name)
	if len(runes) == 1 {
		return tea.KeyPressMsg{Code: runes[0], Text: name}
	}
	return tea.KeyPressMsg{Text: name}
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func viewText(m Model) string {
	return m.View().Content
}

func TestLoadShowsStatusGlyphs(t *testing.T) {
	f := newFixture(t)
	f.env.Insert(t, "alpha", "alpha-value")
	f.env.Insert(t, "beta", "beta-value")
	f.env.Insert(t, "notes/wifi", "wifi-pass")
	f.env.Insert(t, "old/token", "stale")
	f.state.Entries["alpha"] = syncer.EntryState{SyncedAt: f.now.Add(-2 * time.Hour)}
	f.state.Entries["old/token"] = syncer.EntryState{SyncedAt: f.now.Add(-2 * time.Hour)}
	m := f.loadModel(t)
	view := viewText(m)
	for _, want := range []string{"alpha", "beta", "notes/wifi", "old/token", "●", "◐", "✕", "○"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestCursorMoveDoesNotDecrypt(t *testing.T) {
	f := newFixture(t)
	f.env.Insert(t, "alpha", "alpha-value")
	f.env.Insert(t, "beta", "beta-value")
	recorder := f.env.Runner()
	f.deps.Store = &store.Store{Runner: recorder, Dir: f.env.Paths.StoreDir}
	m := f.loadModel(t)
	m = update(t, m, press("j"))
	m = update(t, m, press("k"))
	for _, argv := range recorder.Argv() {
		if strings.Contains(argv, "pass show") {
			t.Fatalf("cursor movement ran %q; must not decrypt", argv)
		}
	}
	view := viewText(m)
	if !strings.Contains(view, "press r to reveal") {
		t.Errorf("detail pane should show the masked placeholder:\n%s", view)
	}
	if strings.Contains(view, "alpha-value") {
		t.Error("view leaked the decrypted value without a reveal")
	}
}

func TestRevealShowsValueThenTickRemasks(t *testing.T) {
	f := newFixture(t)
	f.env.Insert(t, "alpha", "alpha-value")
	m := f.loadModel(t)
	next, cmd := m.Update(press("r"))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("r should return a reveal command")
	}
	reveal, ok := revealFromBatch(t, cmd)
	if !ok {
		t.Fatalf("reveal command returned %T", cmd())
	}
	m = update(t, m, reveal)
	if m.reveal == nil || m.reveal.value != "alpha-value" {
		t.Fatalf("reveal = %+v, want alpha-value", m.reveal)
	}
	if !strings.Contains(viewText(m), "alpha-value") {
		t.Error("view should show the revealed value")
	}
	m = update(t, m, tickMsg{at: f.now.Add(revealDuration)})
	if m.reveal != nil {
		t.Error("tick after 10 s should re-mask")
	}
	if strings.Contains(viewText(m), "alpha-value") {
		t.Error("view should be re-masked after the tick")
	}
}

// revealFromBatch runs a reveal command batch and returns the revealMsg.
func revealFromBatch(t *testing.T, cmd tea.Cmd) (revealMsg, bool) {
	t.Helper()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		return revealMsg{}, false
	}
	for _, inner := range batch {
		if msg, ok := inner().(revealMsg); ok {
			return msg, true
		}
	}
	return revealMsg{}, false
}

func TestRevealClearedOnViewSwitch(t *testing.T) {
	f := newFixture(t)
	f.env.Insert(t, "alpha", "alpha-value")
	m := f.loadModel(t)
	m.reveal = &revealState{name: "alpha", value: "alpha-value", deadline: f.now.Add(time.Minute)}
	m = update(t, m, press("2"))
	if m.reveal != nil {
		t.Error("switching view must clear the reveal")
	}
}
