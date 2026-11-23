package cli

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

var auditedGithub = mapping.Secret{Pass: "work/github", Ref: "op://Private/GitHub/token", Env: "GITHUB_TOKEN", Audit: true}

// auditActions returns the action of each event in the audit log, in order.
func auditActions(t *testing.T, f *cliFixture) []audit.Action {
	t.Helper()
	var actions []audit.Action
	for _, event := range auditEvents(t, f) {
		actions = append(actions, event.Action)
	}
	return actions
}

func auditEvents(t *testing.T, f *cliFixture) []audit.Event {
	t.Helper()
	events, err := audit.Read(f.env.Paths.Audit())
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func TestGetPrintsExactBytesAndRecordsUseViaGet(t *testing.T) {
	f := newCliFixture(t, "")
	f.app.via = "get"
	f.writeMapping(t, auditedGithub)
	f.env.Insert(t, "work/github", "ghp-it's\n")

	if err := f.run(f.app.getCommand(), "work/github"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if f.out.String() != "ghp-it's\n" {
		t.Fatalf("out = %q", f.out.String())
	}
	events := auditEvents(t, f)
	if len(events) != 1 || events[0].Action != audit.ActionUse || events[0].Via != "get" || events[0].Entry != "work/github" {
		t.Fatalf("events = %+v", events)
	}
}

func TestGetUnknownEntryFails(t *testing.T) {
	f := newCliFixture(t, "")
	f.app.via = "get"

	err := f.run(f.app.getCommand(), "nope")
	if err == nil || err.Error() != "unknown pass entry: nope" {
		t.Fatalf("err = %v", err)
	}
}

func TestFirstLoadMappingWithAuditedEntryRecordsAuditOn(t *testing.T) {
	f := newCliFixture(t, "")
	f.app.via = "status"
	f.writeMapping(t, auditedGithub, pagerduty)

	if _, err := f.app.loadMapping(); err != nil {
		t.Fatal(err)
	}
	events := auditEvents(t, f)
	if len(events) != 1 || events[0].Action != audit.ActionAuditOn || events[0].Entry != "work/github" {
		t.Fatalf("events = %+v", events)
	}
}

func TestSyncOverwriteOfAuditedEntryRecordsCompareThenWrite(t *testing.T) {
	f := newCliFixture(t, "o\n")
	f.app.via = "sync"
	audited := pagerduty
	audited.Audit = true
	f.writeMapping(t, audited)
	f.env.SetOp(t, pagerduty.Ref, "new-value")
	f.env.SetOpSignedIn(t, true)
	f.env.Insert(t, "pagerduty-api-key", "old-value")

	if err := f.run(f.app.syncCommand()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	want := []audit.Action{audit.ActionAuditOn, audit.ActionCompare, audit.ActionWrite}
	if got := auditActions(t, f); !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	if detail := auditEvents(t, f)[2].Detail; !strings.HasPrefix(detail, "changed · commit ") {
		t.Fatalf("write detail = %q", detail)
	}
}

func TestRunOfAuditedEntryRecordsUseWithChildName(t *testing.T) {
	f, _ := newRunFixture(t)
	f.app.via = "run"
	f.writeMapping(t, auditedGithub)
	f.env.Insert(t, "work/github", "ghp-value")

	if err := f.run(f.app.runCommand(), "--entry", "work/github", "--", "sh", "-c", "true"); err != nil {
		t.Fatalf("run: %v", err)
	}
	events := auditEvents(t, f)
	if len(events) != 2 || events[1].Action != audit.ActionUse || events[1].Detail != "sh" || events[1].Via != "run" {
		t.Fatalf("events = %+v", events)
	}
}

func TestEnvWithAuditedGlobalRecordsExport(t *testing.T) {
	f := newCliFixture(t, "")
	f.app.via = "env"
	f.writeMapping(t, auditedGithub)
	f.env.Insert(t, "work/github", "ghp-value")

	if err := f.run(f.app.envCommand(), "--zsh"); err != nil {
		t.Fatalf("env: %v", err)
	}
	want := []audit.Action{audit.ActionAuditOn, audit.ActionExport}
	if got := auditActions(t, f); !slices.Equal(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	if got := evalInZsh(t, f.out.String(), "GITHUB_TOKEN"); got != "ghp-value" {
		t.Fatalf("GITHUB_TOKEN = %q", got)
	}
}

func TestRemoveAfterAuditedEntryLeftMappingRecordsDelete(t *testing.T) {
	f := newCliFixture(t, "")
	f.app.via = "tui"
	f.writeMapping(t, auditedGithub)
	f.env.Insert(t, "work/github", "ghp-value")
	if _, err := f.app.loadMapping(); err != nil {
		t.Fatal(err)
	}
	if err := mapping.RemoveSecret(f.env.Paths.Mapping(), "work/github"); err != nil {
		t.Fatal(err)
	}

	if err := f.app.store().Remove(t.Context(), "work/github"); err != nil {
		t.Fatal(err)
	}
	if got := auditActions(t, f); !slices.Equal(got, []audit.Action{audit.ActionAuditOn, audit.ActionDelete}) {
		t.Fatalf("actions = %v, want [audit-on delete]", got)
	}
}

func TestGetWithBrokenMappingStillRecordsUseOfAuditedEntry(t *testing.T) {
	f := newCliFixture(t, "")
	f.app.via = "get"
	f.writeMapping(t, auditedGithub)
	f.env.Insert(t, "work/github", "ghp-value")
	if _, err := f.app.loadMapping(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.env.Paths.Mapping(), []byte("version = 1\n[[secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := f.run(f.app.getCommand(), "work/github"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := auditActions(t, f); !slices.Equal(got, []audit.Action{audit.ActionAuditOn, audit.ActionUse}) {
		t.Fatalf("actions = %v, want [audit-on use]", got)
	}
}
