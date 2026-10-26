package cli

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

// evalInZsh sources script in a real zsh and prints the variable, proving
// the quoting survives evaluation.
func evalInZsh(t *testing.T, script, variable string) string {
	t.Helper()
	out, err := exec.Command("zsh", "-c", script+"\nprintf %s \"$"+variable+"\"").Output()
	if err != nil {
		t.Fatalf("zsh: %v", err)
	}
	return string(out)
}

func TestEnvValueWithQuotesAndDollar(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	f.env.Insert(t, "pagerduty-api-key", "a'b$(x)")

	if err := f.run(f.app.envCommand(), "--zsh"); err != nil {
		t.Fatalf("env: %v", err)
	}
	if got := evalInZsh(t, f.out.String(), "PAGERDUTY_API_KEY"); got != "a'b$(x)" {
		t.Fatalf("evaluated value = %q", got)
	}
}

func TestEnvSkipsMissingEntryWithWarning(t *testing.T) {
	other := mapping.Secret{Pass: "other-token", Ref: "op://Private/Other/token", Env: "OTHER_TOKEN"}
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty, other)
	f.env.Insert(t, "other-token", "other-value")

	if err := f.run(f.app.envCommand(), "--zsh"); err != nil {
		t.Fatalf("env: %v", err)
	}
	if !strings.Contains(f.errOut.String(), "pvault: skipped pagerduty-api-key") {
		t.Fatalf("missing warning on stderr:\n%s", f.errOut.String())
	}
	if got := evalInZsh(t, f.out.String(), "OTHER_TOKEN"); got != "other-value" {
		t.Fatalf("evaluated OTHER_TOKEN = %q", got)
	}
	if strings.Contains(f.out.String(), "PAGERDUTY_API_KEY") {
		t.Fatalf("skipped entry still exported:\n%s", f.out.String())
	}
}

func TestEnvMissingMappingPrintsNothing(t *testing.T) {
	f := newCliFixture(t, "")
	if err := f.run(f.app.envCommand(), "--zsh"); err != nil {
		t.Fatalf("env: %v", err)
	}
	if f.out.String() != "" || f.errOut.String() != "" {
		t.Fatalf("out = %q, errOut = %q", f.out.String(), f.errOut.String())
	}
}
