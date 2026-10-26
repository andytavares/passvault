package cli

import (
	"os/exec"
	"strings"
	"testing"
)

// headRevision returns the store's current HEAD hash.
func headRevision(t *testing.T, f *cliFixture) string {
	t.Helper()
	out, err := exec.Command("git", "-C", f.env.Paths.StoreDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestRollbackYesRestoresOlderValue(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	if err := f.syncOnce(t, "first-value"); err != nil {
		t.Fatalf("sync 1: %v", err)
	}
	first := headRevision(t, f)
	f.tick()
	f.app.in = strings.NewReader("o\n")
	if err := f.syncOnce(t, "second-value"); err != nil {
		t.Fatalf("sync 2: %v", err)
	}
	f.out.Reset()

	err := f.run(f.app.rollbackCommand(), first, "--yes")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got := f.env.Show(t, "pagerduty-api-key"); got != "first-value" {
		t.Fatalf("pass value = %q, want first-value", got)
	}
	if !strings.Contains(f.out.String(), "undo with pvault rollback HEAD~1") {
		t.Fatalf("missing undo hint:\n%s", f.out.String())
	}
}

func TestRollbackDeclinedChangesNothing(t *testing.T) {
	f := newCliFixture(t, "n\n")
	f.writeMapping(t, pagerduty)
	if err := f.syncOnce(t, "first-value"); err != nil {
		t.Fatalf("sync 1: %v", err)
	}
	first := headRevision(t, f)
	f.tick()
	f.app.in = strings.NewReader("o\n")
	if err := f.syncOnce(t, "second-value"); err != nil {
		t.Fatalf("sync 2: %v", err)
	}
	before := commitCount(t, f.env)
	f.app.in = strings.NewReader("n\n")

	err := f.run(f.app.rollbackCommand(), first)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got := f.env.Show(t, "pagerduty-api-key"); got != "second-value" {
		t.Fatalf("pass value = %q, want second-value", got)
	}
	if after := commitCount(t, f.env); after != before {
		t.Fatalf("declined rollback committed: %d → %d", before, after)
	}
}

func TestRollbackNoopSaysSo(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	if err := f.syncOnce(t, "only-value"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	head := headRevision(t, f)

	err := f.run(f.app.rollbackCommand(), head, "--yes")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if !strings.Contains(f.out.String(), "Nothing to roll back") {
		t.Fatalf("output:\n%s", f.out.String())
	}
}
