package cli

import (
	"strings"
	"testing"
)

func TestCheckDifferingExits1AndWritesNothing(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	f.env.SetOp(t, pagerduty.Ref, "new-value")
	f.env.SetOpSignedIn(t, true)
	f.env.Insert(t, "pagerduty-api-key", "old-value")
	before := commitCount(t, f.env)

	err := f.run(f.app.checkCommand())
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, err %v", code, err)
	}
	if !strings.Contains(f.out.String(), "~ pagerduty-api-key") {
		t.Fatalf("output missing differs line:\n%s", f.out.String())
	}
	if after := commitCount(t, f.env); after != before {
		t.Fatalf("check wrote commits: %d → %d", before, after)
	}
	if got := f.env.Show(t, "pagerduty-api-key"); got != "old-value" {
		t.Fatalf("check changed the value: %q", got)
	}
}

func TestCheckEqualExits0(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	if err := f.syncOnce(t, "same-value"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	err := f.run(f.app.checkCommand())
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit code = %d, err %v", code, err)
	}
	if !strings.Contains(f.out.String(), "= pagerduty-api-key") {
		t.Fatalf("output missing unchanged line:\n%s", f.out.String())
	}
}

func TestCheckMissingEntryExits1(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	f.env.SetOp(t, pagerduty.Ref, "some-value")
	f.env.SetOpSignedIn(t, true)

	err := f.run(f.app.checkCommand())
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, err %v", code, err)
	}
	if !strings.Contains(f.out.String(), "+ pagerduty-api-key") {
		t.Fatalf("output missing add line:\n%s", f.out.String())
	}
}
