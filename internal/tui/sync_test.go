package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

// startSync runs the real syncCmd in the background; conflicts open the
// modal on m's shared conflictBox, as program.Send would in the live TUI.
func startSync(f *fixture, m *Model) chan tea.Msg {
	m.syncing = true
	m.active = viewSync
	m.conflictHook = func(msg tea.Msg) {
		conflict := msg.(conflictMsg)
		m.conflict.set(&conflictState{conflict: conflict.conflict, answer: conflict.answer, reveal: conflict.reveal})
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- syncCmd(m)() }()
	return done
}

// openModal waits until the decider has opened the conflict modal.
func openModal(t *testing.T, m *Model) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for m.conflict.get() == nil {
		if time.Now().After(deadline) {
			t.Fatal("sync never raised a conflict")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestConflictSkipKeepsPass(t *testing.T) {
	f := newFixture(t)
	seedConflict(t, f)
	m := f.loadModel(t)
	done := startSync(f, &m)
	openModal(t, &m)
	if m.conflict.get().conflict.Secret.Pass != "alpha" {
		t.Fatalf("conflict on %q, want alpha", m.conflict.get().conflict.Secret.Pass)
	}
	m.conflict.get().answer <- syncer.DecisionSkip
	report := <-done
	if got := f.env.Show(t, "alpha"); got != "old-pass-value" {
		t.Errorf("skip changed pass to %q", got)
	}
	finished, ok := report.(syncDoneMsg)
	if !ok {
		t.Fatalf("sync returned %T", report)
	}
	if finished.skipped != 1 {
		t.Errorf("skipped = %d, want 1", finished.skipped)
	}
}

func TestConflictOverwriteWritesPass(t *testing.T) {
	f := newFixture(t)
	seedConflict(t, f)
	m := f.loadModel(t)
	done := startSync(f, &m)
	openModal(t, &m)
	m.conflict.get().answer <- syncer.DecisionOverwrite
	<-done
	if got := f.env.Show(t, "alpha"); got != "alpha-value" {
		t.Errorf("overwrite left pass at %q", got)
	}
}

func TestConflictModalKeysAndNoOverwriteAll(t *testing.T) {
	f := newFixture(t)
	seedConflict(t, f)
	m := f.loadModel(t)
	done := startSync(f, &m)
	openModal(t, &m)
	view := viewText(m)
	for _, want := range []string{"[o] overwrite", "[s] skip", "[enter] show both", "[esc] stop"} {
		if !strings.Contains(view, want) {
			t.Errorf("modal missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "all") {
		t.Errorf("modal must not offer overwrite-all:\n%s", view)
	}
	if _, ok := conflictKeys["a"]; ok {
		t.Error("modal must not bind a key that overwrites all")
	}
	m.conflict.get().answer <- syncer.DecisionQuit
	<-done
}

// seedConflict makes alpha differ between 1Password and pass.
func seedConflict(t *testing.T, f *fixture) {
	t.Helper()
	f.env.Insert(t, "alpha", "old-pass-value")
	f.env.Insert(t, "beta", "beta-value")
	f.state.Entries["alpha"] = syncer.EntryState{SyncedAt: f.now.Add(-time.Hour)}
	f.state.Entries["beta"] = syncer.EntryState{SyncedAt: f.now.Add(-time.Hour)}
}
