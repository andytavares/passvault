package cli

import (
	"strings"
	"testing"
)

func TestHistoryListsSyncCommitsAndTags(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	if err := f.syncOnce(t, "first-value"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	err := f.run(f.app.historyCommand())
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	out := f.out.String()
	if !strings.Contains(out, "Sync tags: sync-20261025T130512") {
		t.Fatalf("missing sync tag header:\n%s", out)
	}
	if !strings.Contains(out, "pagerduty-api-key") {
		t.Fatalf("missing sync commit:\n%s", out)
	}
}

func TestHistoryForOneEntry(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	if err := f.syncOnce(t, "first-value"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	err := f.run(f.app.historyCommand(), "pagerduty-api-key")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	out := f.out.String()
	if strings.Contains(out, "Sync tags:") {
		t.Fatalf("named history must not print the tag header:\n%s", out)
	}
	if !strings.Contains(out, "pagerduty-api-key") {
		t.Fatalf("missing entry commit:\n%s", out)
	}
}
