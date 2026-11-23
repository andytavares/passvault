package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
)

const auditHeader = "TIME                 ENTRY       ACTION     VIA       USER      WHAT"

// appendAuditEvent records the Entry and Action of event at event.Time through
// the real Recorder, so the log keeps a valid hash chain.
func appendAuditEvent(t *testing.T, f *cliFixture, event audit.Event) {
	t.Helper()
	recorder := &audit.Recorder{
		Path:    f.env.Paths.Audit(),
		Via:     "get",
		Now:     func() time.Time { return event.Time },
		Audited: func(string) bool { return true },
		Callers: func() []audit.Process { return []audit.Process{{PID: 4242, Comm: "zsh", Argv: []string{"-zsh"}}} },
	}
	if err := recorder.Record(event.Entry, event.Action, ""); err != nil {
		t.Fatal(err)
	}
}

// newAuditFixture writes three events: work/github 3h ago, aws 30m ago,
// work/github 10m ago, relative to the fixture clock.
func newAuditFixture(t *testing.T) (*cliFixture, []time.Time) {
	t.Helper()
	f := newCliFixture(t, "")
	now := f.app.now()
	times := []time.Time{now.Add(-3 * time.Hour), now.Add(-30 * time.Minute), now.Add(-10 * time.Minute)}
	appendAuditEvent(t, f, audit.Event{Entry: "work/github", Action: audit.ActionUse, Time: times[0]})
	appendAuditEvent(t, f, audit.Event{Entry: "aws", Action: audit.ActionView, Time: times[1]})
	appendAuditEvent(t, f, audit.Event{Entry: "work/github", Action: audit.ActionCopy, Time: times[2]})
	return f, times
}

func auditTime(at time.Time) string { return at.Local().Format("2006-01-02 15:04:05") }

func outputLines(f *cliFixture) []string {
	return strings.Split(strings.TrimSuffix(f.out.String(), "\n"), "\n")
}

func TestAuditListsEventsNewestFirstWithFooter(t *testing.T) {
	f, times := newAuditFixture(t)

	if err := f.run(f.app.auditCommand()); err != nil {
		t.Fatalf("audit: %v", err)
	}
	lines := outputLines(f)
	if len(lines) != 5 || lines[0] != auditHeader {
		t.Fatalf("out = %q", f.out.String())
	}
	if !strings.HasPrefix(lines[1], auditTime(times[2])+"  work/github copy       get       ") || !strings.HasSuffix(lines[1], " -zsh") {
		t.Errorf("first row = %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], auditTime(times[1])+"  aws         view       get       ") {
		t.Errorf("second row = %q", lines[2])
	}
	if !strings.HasPrefix(lines[3], auditTime(times[0])+"  work/github use        get       ") {
		t.Errorf("third row = %q", lines[3])
	}
	if want := "3 events · " + f.env.Paths.Audit(); lines[4] != want {
		t.Errorf("footer = %q, want %q", lines[4], want)
	}
}

func TestAuditWithNameListsOnlyThatEntry(t *testing.T) {
	f, times := newAuditFixture(t)

	if err := f.run(f.app.auditCommand(), "aws"); err != nil {
		t.Fatalf("audit: %v", err)
	}
	lines := outputLines(f)
	if len(lines) != 3 || !strings.HasPrefix(lines[1], auditTime(times[1])+"  aws ") {
		t.Fatalf("out = %q", f.out.String())
	}
	if want := "1 events · " + f.env.Paths.Audit(); lines[2] != want {
		t.Errorf("footer = %q, want %q", lines[2], want)
	}
}

func TestAuditWithSinceListsOnlyRecentEvents(t *testing.T) {
	f, times := newAuditFixture(t)

	if err := f.run(f.app.auditCommand(), "--since", "1h"); err != nil {
		t.Fatalf("audit: %v", err)
	}
	lines := outputLines(f)
	if len(lines) != 4 || !strings.HasPrefix(lines[1], auditTime(times[2])) || !strings.HasPrefix(lines[2], auditTime(times[1])) {
		t.Fatalf("out = %q", f.out.String())
	}
}

func TestAuditWithInvalidSinceNamesTheValue(t *testing.T) {
	f, _ := newAuditFixture(t)

	err := f.run(f.app.auditCommand(), "--since", "soon")
	if err == nil || !strings.Contains(err.Error(), `"soon"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestAuditJSONPrintsOneEventPerLineNewestFirst(t *testing.T) {
	f, times := newAuditFixture(t)

	if err := f.run(f.app.auditCommand(), "--json"); err != nil {
		t.Fatalf("audit: %v", err)
	}
	var events []audit.Event
	for _, line := range outputLines(f) {
		var event audit.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		events = append(events, event)
	}
	if len(events) != 3 || events[0].Action != audit.ActionCopy || events[1].Entry != "aws" || !events[2].Time.Equal(times[0]) {
		t.Fatalf("events = %+v", events)
	}
}

func TestAuditVerifyPrintsIntactChain(t *testing.T) {
	f, _ := newAuditFixture(t)

	if err := f.run(f.app.auditCommand(), "--verify"); err != nil {
		t.Fatalf("audit --verify: %v", err)
	}
	if f.out.String() != "OK, 3 events, chain intact\n" {
		t.Fatalf("out = %q", f.out.String())
	}
}

func TestAuditVerifyAfterEditingFirstLineFailsNamingLineTwo(t *testing.T) {
	f, _ := newAuditFixture(t)
	path := f.env.Paths.Audit()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Replace(data, []byte(`"work/github"`), []byte(`"work/gitlab"`), 1), 0o600); err != nil {
		t.Fatal(err)
	}

	err = f.run(f.app.auditCommand(), "--verify")
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestAuditWithEmptyLogPrintsNoEvents(t *testing.T) {
	f := newCliFixture(t, "")

	if err := f.run(f.app.auditCommand()); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if f.out.String() != "No audit events\n" {
		t.Fatalf("out = %q", f.out.String())
	}
}
