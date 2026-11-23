package tui

import (
	"fmt"
	"os"
	"os/user"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
)

const auditedMapping = `version = 1

[[secret]]
pass = "alpha"
ref = "op://Private/Alpha/credential"
env = "ALPHA_KEY"
audit = true

[[secret]]
pass = "beta"
ref = "op://Private/Beta/credential"
`

// recordUse appends a use event with event's time, entry, via, detail and
// callers to the fixture's audit log.
func recordUse(t *testing.T, f *fixture, event audit.Event) {
	t.Helper()
	recorder := &audit.Recorder{
		Path: f.env.Paths.Audit(), Via: event.Via,
		Now:     func() time.Time { return event.Time },
		Audited: func(string) bool { return true },
		Callers: func() []audit.Process { return event.Callers },
	}
	if err := recorder.Record(event.Entry, audit.ActionUse, event.Detail); err != nil {
		t.Fatal(err)
	}
}

// openAudit switches from the Env view to the Audit view and loads the log.
func openAudit(t *testing.T, m Model) Model {
	t.Helper()
	m.active = viewEnv
	next, cmd := m.Update(press("tab"))
	return update(t, next.(Model), cmd())
}

func lineWith(text, needle string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

func TestVaultMarksAuditedEntryAndShowsAuditRow(t *testing.T) {
	f := newFixture(t)
	writeMapping(t, f.deps.MappingPath, auditedMapping)
	f.env.Insert(t, "alpha", "alpha-value")
	f.env.Insert(t, "beta", "beta-value")
	view := ansi.Strip(viewText(f.loadModel(t)))
	alpha, beta := lineWith(view, "▸ alpha"), lineWith(view, "  beta")
	if !strings.Contains(alpha, "◉ ◐") {
		t.Errorf("alpha row lacks the audit mark: %q\n%s", alpha, view)
	}
	if strings.Contains(beta, "◉") {
		t.Errorf("beta is not audited but shows ◉: %q", beta)
	}
	if glyphColumn(alpha) != glyphColumn(beta) {
		t.Errorf("sync glyphs are not aligned:\n%q\n%q", alpha, beta)
	}
	if !strings.Contains(view, "Audit      on · reveal and copy are logged") {
		t.Errorf("detail pane lacks the Audit row:\n%s", view)
	}
	if !strings.Contains(view, "Vault   Sync   History   Mapping   Env   Audit") {
		t.Errorf("tab bar lacks Audit:\n%s", view)
	}
}

func TestVaultDetailOmitsAuditRowForUnauditedEntry(t *testing.T) {
	f := newFixture(t)
	f.env.Insert(t, "alpha", "alpha-value")
	view := ansi.Strip(viewText(f.loadModel(t)))
	if strings.Contains(view, "reveal and copy are logged") {
		t.Errorf("unaudited alpha shows the Audit row:\n%s", view)
	}
}

func TestEnvAuditColumnShowsOnOrDash(t *testing.T) {
	f := newFixture(t)
	writeMapping(t, f.deps.MappingPath, auditedMapping)
	view := ansi.Strip(viewText(envModel(t, f)))
	if !strings.Contains(view, "ENTRY ENV       SCOPE  STATE AUDIT SOURCE") {
		t.Errorf("header lacks AUDIT after STATE:\n%s", view)
	}
	if line := lineWith(view, "▸ alpha"); !strings.Contains(line, "● on  ◉ on  1Password") {
		t.Errorf("alpha row = %q", line)
	}
	if line := lineWith(view, "  beta"); !strings.Contains(line, "—     —     1Password") {
		t.Errorf("beta row = %q", line)
	}
	if !strings.Contains(view, "A audit on/off") {
		t.Errorf("footer lacks the A key:\n%s", view)
	}
}

func TestEnvAuditKeyWritesAndRemovesAuditFlag(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = runKey(t, m, press("A"))
	content, _ := os.ReadFile(f.deps.MappingPath)
	if !strings.Contains(string(content), "audit = true") || m.notice != "audit alpha on" {
		t.Fatalf("first A: notice %q, mapping:\n%s", m.notice, content)
	}
	m = runKey(t, m, press("A"))
	content, _ = os.ReadFile(f.deps.MappingPath)
	if strings.Contains(string(content), "audit") || m.notice != "audit alpha off" {
		t.Fatalf("second A: notice %q, mapping:\n%s", m.notice, content)
	}
}

func TestEnvAuditKeyOnUnmappedEntryDoesNothing(t *testing.T) {
	f := newFixture(t)
	m := envModel(t, f)
	m = update(t, update(t, m, press("j")), press("j"))
	before, _ := os.ReadFile(f.deps.MappingPath)
	next, cmd := m.Update(press("A"))
	after, _ := os.ReadFile(f.deps.MappingPath)
	if cmd != nil || next.(Model).notice != "" || string(before) != string(after) {
		t.Fatalf("A on zeta: cmd=%v notice=%q mapping:\n%s", cmd != nil, next.(Model).notice, after)
	}
}

// stripeKeyAudit opens the Audit view over two stripe-key events; the newer
// one has callers and a detail.
func stripeKeyAudit(t *testing.T) string {
	t.Helper()
	f := newFixture(t)
	callers := []audit.Process{
		{PID: 48212, Comm: "zsh", Argv: []string{"/bin/zsh", "./deploy.sh"}},
		{PID: 40011, Comm: "-zsh"},
	}
	recordUse(t, f, audit.Event{Time: time.Date(2026, 11, 23, 13, 41, 0, 0, time.UTC), Entry: "stripe-key", Via: "run"})
	recordUse(t, f, audit.Event{Time: time.Date(2026, 11, 23, 14, 2, 11, 0, time.UTC), Entry: "stripe-key",
		Via: "get", Detail: "env STRIPE_KEY", Callers: callers})
	return ansi.Strip(viewText(openAudit(t, f.loadModel(t))))
}

func TestAuditViewListsEventsNewestFirst(t *testing.T) {
	view := stripeKeyAudit(t)
	for _, want := range []string{
		"Audit ─ all entries · 2 events",
		"▸ 11-23 14:02  stripe-key  use      get",
		"  11-23 13:41  stripe-key  use      run",
		"j/k move • f filter entry • tab next view",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Index(view, "14:02  stripe-key") > strings.Index(view, "13:41  stripe-key") {
		t.Errorf("events not newest first:\n%s", view)
	}
}

func TestAuditViewDetailShowsSelectedEvent(t *testing.T) {
	view := stripeKeyAudit(t)
	current, _ := user.Current()
	host, _ := os.Hostname()
	for _, want := range []string{
		"│ 2026-11-23 14:02:11 UTC",
		"│ Entry   stripe-key",
		"│ Action  use  via get",
		fmt.Sprintf("│ Who     %s (uid %d) on %s", current.Username, os.Getuid(), host),
		"│ Detail  env STRIPE_KEY",
		"│ What    48212  zsh ./deploy.sh",
		"│         40011  -zsh",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestAuditFilterCyclesThroughEntriesThenAll(t *testing.T) {
	f := newFixture(t)
	recordUse(t, f, audit.Event{Time: time.Date(2026, 11, 23, 13, 0, 0, 0, time.UTC), Entry: "beta", Via: "get"})
	recordUse(t, f, audit.Event{Time: time.Date(2026, 11, 23, 14, 0, 0, 0, time.UTC), Entry: "alpha", Via: "get"})
	recordUse(t, f, audit.Event{Time: time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC), Entry: "alpha", Via: "run"})
	m := openAudit(t, f.loadModel(t))

	m = update(t, m, press("f"))
	view := ansi.Strip(viewText(m))
	if !strings.Contains(view, "Audit ─ alpha · 2 events") || strings.Contains(view, "  beta  ") {
		t.Fatalf("first f should show alpha only:\n%s", view)
	}
	m = update(t, m, press("f"))
	view = ansi.Strip(viewText(m))
	if !strings.Contains(view, "Audit ─ beta · 1 events") || !strings.Contains(view, "▸ 11-23 13:00  beta") {
		t.Fatalf("second f should show beta only:\n%s", view)
	}
	m = update(t, m, press("f"))
	if view = ansi.Strip(viewText(m)); !strings.Contains(view, "Audit ─ all entries · 3 events") {
		t.Fatalf("third f should show all entries:\n%s", view)
	}
}

func TestAuditViewEmptyLogShowsHint(t *testing.T) {
	f := newFixture(t)
	view := ansi.Strip(viewText(openAudit(t, f.loadModel(t))))
	if !strings.Contains(view, "No audit events. Mark a secret with A in Env, or audit = true in mapping.toml.") {
		t.Errorf("empty Audit view lacks the hint:\n%s", view)
	}
}

func TestAuditViewMovesCursorWithJAndK(t *testing.T) {
	f := newFixture(t)
	recordUse(t, f, audit.Event{Time: time.Date(2026, 11, 23, 13, 0, 0, 0, time.UTC), Entry: "beta", Via: "get"})
	recordUse(t, f, audit.Event{Time: time.Date(2026, 11, 23, 14, 0, 0, 0, time.UTC), Entry: "alpha", Via: "get"})
	m := update(t, openAudit(t, f.loadModel(t)), press("j"))
	view := ansi.Strip(viewText(m))
	if !strings.Contains(view, "▸ 11-23 13:00  beta") || !strings.Contains(view, "│ Entry   beta") {
		t.Fatalf("j should select the older beta event:\n%s", view)
	}
	m = update(t, m, press("k"))
	if view = ansi.Strip(viewText(m)); !strings.Contains(view, "│ Entry   alpha") {
		t.Fatalf("k should select alpha again:\n%s", view)
	}
}

// glyphColumn is the rune index of the sync glyph in a Vault row.
func glyphColumn(line string) int {
	return slices.Index([]rune(line), '◐')
}

func TestVaultTruncatesLongNameAndKeepsColumnsAligned(t *testing.T) {
	f := newFixture(t)
	long := "terminator-app-specific-password"
	writeMapping(t, f.deps.MappingPath, fmt.Sprintf(`version = 1

[[secret]]
pass = "linear-api-key"
ref = "op://Private/Linear/credential"
audit = true

[[secret]]
pass = %q
ref = "op://Private/Terminator/credential"
audit = true
`, long))
	f.env.Insert(t, "linear-api-key", "linear-value")
	f.env.Insert(t, long, "terminator-value")
	view := ansi.Strip(viewText(f.loadModel(t)))
	short, longLine := lineWith(view, "▸ linear-api-key"), lineWith(view, "terminator-app")
	auditColumn := func(line string) int { return slices.Index([]rune(line), '◉') }
	if auditColumn(short) != auditColumn(longLine) {
		t.Errorf("audit marks are not aligned:\n%q\n%q", short, longLine)
	}
	if !strings.Contains(longLine, "terminator-app-specific-pa… ◉") {
		t.Errorf("long name is not truncated with a gap before the mark: %q", longLine)
	}
}
