package cli

import (
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

var auditedGhCommand = mapping.Secret{Pass: "work/gh-cli", Ref: "op://Private/GitHub/cli", Env: "GH_TOKEN", Scope: mapping.ScopeCommand, Commands: []string{"gh"}, Audit: true}

func TestPreexecRecordsInheritOnlyForAuditedGlobal(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "github-value")
	t.Setenv("GH_TOKEN", "gh-value")
	f := newCliFixture(t, "")
	f.app.via = "preexec"
	f.writeMapping(t, auditedGithub, pagerduty, auditedGhCommand)

	if err := f.run(f.app.preexecCommand(), "--", "npm", "run", "dev"); err != nil {
		t.Fatalf("preexec: %v", err)
	}
	events := auditEvents(t, f)
	if len(events) != 1 || events[0].Entry != "work/github" || events[0].Action != audit.ActionInherit ||
		events[0].Detail != "npm run dev" || events[0].Via != "preexec" {
		t.Fatalf("events = %+v", events)
	}
	if err := f.app.runAuditVerify(); err != nil {
		t.Fatalf("audit --verify: %v", err)
	}
}

func TestPreexecReplacesKnownEnvValueWithItsName(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test_value_1234567890")
	f := newCliFixture(t, "")
	f.app.via = "preexec"
	f.writeMapping(t, auditedGithub)

	if err := f.run(f.app.preexecCommand(), "--", "curl", "-H", "x: ghp_test_value_1234567890"); err != nil {
		t.Fatalf("preexec: %v", err)
	}
	events := auditEvents(t, f)
	if len(events) != 1 || !strings.Contains(events[0].Detail, "[GITHUB_TOKEN]") || strings.Contains(events[0].Detail, "ghp_test_value") {
		t.Fatalf("events = %+v", events)
	}
}

func TestPreexecSkipsAuditedGlobalNotSetInThisShell(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	f := newCliFixture(t, "")
	f.app.via = "preexec"
	f.writeMapping(t, auditedGithub)

	if err := f.run(f.app.preexecCommand(), "--", "npm", "run", "dev"); err != nil {
		t.Fatalf("preexec: %v", err)
	}
	if events := auditEvents(t, f); len(events) != 0 {
		t.Fatalf("events = %+v, want none: the shell never received GITHUB_TOKEN", events)
	}
}

func TestPreexecWithoutMappingRecordsNothing(t *testing.T) {
	f := newCliFixture(t, "")
	f.app.via = "preexec"

	if err := f.run(f.app.preexecCommand(), "--", "npm", "run", "dev"); err != nil {
		t.Fatalf("preexec: %v", err)
	}
	if events := auditEvents(t, f); len(events) != 0 {
		t.Fatalf("events = %+v", events)
	}
}

func TestEnvWithAuditedGlobalInstallsPreexecHookInZsh(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, auditedGithub)
	f.env.Insert(t, "work/github", "ghp-value")

	if err := f.run(f.app.envCommand(), "--zsh"); err != nil {
		t.Fatalf("env: %v", err)
	}
	hooks := evalInZsh(t, f.out.String()+"\nhooks=\"$(add-zsh-hook -L preexec)\"", "hooks")
	if !strings.Contains(hooks, "_pvault_preexec") {
		t.Fatalf("preexec hooks = %q", hooks)
	}
}
