package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
)

// applyCmd runs cmd and applies its message, or each message of a batch.
func applyCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return update(t, m, msg)
	}
	for _, inner := range batch {
		if inner != nil {
			m = update(t, m, inner())
		}
	}
	return m
}

// seedAlphaHistory commits alpha once and changes it twice, then commits
// beta and changes it once after the last alpha change.
func seedAlphaHistory(t *testing.T, f *fixture) {
	t.Helper()
	f.env.Insert(t, "alpha", "alpha-v1")
	f.env.Insert(t, "alpha", "alpha-v2")
	f.env.Insert(t, "alpha", "alpha-v3")
	f.env.Insert(t, "beta", "beta-v1")
	f.env.Insert(t, "beta", "beta-v2")
}

// pressHistoryKey presses key and applies the commands it starts.
func pressHistoryKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	next, cmd := m.Update(press(key))
	return applyCmd(t, next.(Model), cmd)
}

func TestVaultHistoryKeyOpensHistoryFilteredToEntry(t *testing.T) {
	f := newFixture(t)
	seedAlphaHistory(t, f)
	m := pressHistoryKey(t, f.loadModel(t), "h")
	view := ansi.Strip(viewText(m))
	if m.active != viewHistory || !strings.Contains(view, "History ─ alpha · 3 changes") {
		t.Fatalf("h should open History filtered to alpha:\n%s", view)
	}
	for i, kind := range []string{"changed", "changed", "added"} {
		revision := m.revisions[i]
		want := fmt.Sprintf("%s %s  %s", revision.Short, revision.Time.Format("01-02 15:04"), kind)
		if !strings.Contains(view, want) {
			t.Errorf("view missing row %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "enter roll back entry • f filter entry") {
		t.Errorf("footer lacks the filtered History keys:\n%s", view)
	}
}

func TestHistoryFilterKeyStepsToNextEntryThenAll(t *testing.T) {
	f := newFixture(t)
	seedAlphaHistory(t, f)
	m := pressHistoryKey(t, pressHistoryKey(t, f.loadModel(t), "h"), "f")
	if view := ansi.Strip(viewText(m)); !strings.Contains(view, "History ─ beta · 2 changes") {
		t.Fatalf("first f should filter to beta:\n%s", view)
	}
	m = pressHistoryKey(t, m, "f")
	view := ansi.Strip(viewText(m))
	if !strings.Contains(view, "History ─ sync tags and commits") || !strings.Contains(view, "enter roll back to • f filter entry") {
		t.Fatalf("second f should show the store-wide history:\n%s", view)
	}
}

func TestFilteredRollbackRestoresOnlyThatEntry(t *testing.T) {
	f := newFixture(t)
	seedAlphaHistory(t, f)
	m := pressHistoryKey(t, f.loadModel(t), "h")
	m = pressHistoryKey(t, update(t, m, press("j")), "enter")
	if prompt := ansi.Strip(viewText(m)); !strings.Contains(prompt, "Rollback alpha to "+m.revisions[1].Short+":") {
		t.Fatalf("prompt should name alpha and the revision:\n%s", prompt)
	}
	m = typeKeys(t, m, "rollback")
	if m.err != nil {
		t.Fatalf("rollback: %v", m.err)
	}
	if alpha, beta := f.env.Show(t, "alpha"), f.env.Show(t, "beta"); alpha != "alpha-v2" || beta != "beta-v2" {
		t.Errorf("after rollback alpha = %q, beta = %q; want alpha-v2, beta-v2", alpha, beta)
	}
}

func TestFilteredHistoryDetailShowsWhoWroteTheCommit(t *testing.T) {
	f := newFixture(t)
	seedAlphaHistory(t, f)
	revisions, err := f.deps.Store.History(context.Background(), "alpha", 1)
	if err != nil {
		t.Fatal(err)
	}
	recordEvent(t, f, audit.Event{Time: time.Date(2026, 11, 23, 16, 41, 0, 0, time.UTC), Entry: "alpha",
		Action: audit.ActionWrite, Via: "tui", Detail: "insert · commit " + revisions[0].Short,
		Callers: []audit.Process{{PID: 4120, Comm: "-zsh"}}})
	m := pressHistoryKey(t, f.loadModel(t), "h")
	view := ansi.Strip(viewText(m))
	current, _ := user.Current()
	host, _ := os.Hostname()
	for _, want := range []string{
		"│ " + revisions[0].Short + " · " + revisions[0].Time.Format("2006-01-02 15:04"),
		"│ Change  changed",
		"│ Commit  Add given password for alpha to store.",
		"│ Tag     -",
		fmt.Sprintf("│ Who     %s (uid %d) on %s via tui", current.Username, os.Getuid(), host),
		"│ What    4120  -zsh",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if view = ansi.Strip(viewText(update(t, m, press("j")))); strings.Contains(view, "Who ") {
		t.Errorf("a revision without a write event shows a Who line:\n%s", view)
	}
}

func TestFilteredHistoryMatchesDeleteEventBySevenCharacterHash(t *testing.T) {
	f := newFixture(t)
	seedAlphaHistory(t, f)
	if out, err := exec.Command("git", "-C", f.env.Paths.StoreDir, "config", "core.abbrev", "12").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	revisions, err := f.deps.Store.History(context.Background(), "alpha", 1)
	if err != nil {
		t.Fatal(err)
	}
	recordEvent(t, f, audit.Event{Time: time.Date(2026, 11, 23, 16, 41, 0, 0, time.UTC), Entry: "alpha",
		Action: audit.ActionDelete, Via: "tui", Detail: "removed · commit " + revisions[0].Hash[:7],
		Callers: []audit.Process{{PID: 4120, Comm: "-zsh"}}})
	view := ansi.Strip(viewText(pressHistoryKey(t, f.loadModel(t), "h")))
	if !strings.Contains(view, "│ What    4120  -zsh") {
		t.Errorf("detail lacks the delete event's caller:\n%s", view)
	}
}

func TestHistoryIgnoresRevisionsLoadedForAnotherEntry(t *testing.T) {
	f := newFixture(t)
	seedAlphaHistory(t, f)
	m := pressHistoryKey(t, f.loadModel(t), "h")
	stale := revisionsCmd(f.deps.Store, "beta")()
	if view := ansi.Strip(viewText(update(t, m, stale))); !strings.Contains(view, "History ─ alpha · 3 changes") {
		t.Errorf("beta's revisions replaced alpha's list:\n%s", view)
	}
}
