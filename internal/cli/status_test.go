package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

func TestStatusNeverForUnsyncedEntry(t *testing.T) {
	unsynced := mapping.Secret{Pass: "work/aws-access-key", Ref: "op://Private/AWS/key", Env: "AWS_ACCESS_KEY_ID"}
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty, unsynced)
	f.env.SetOp(t, pagerduty.Ref, "pd-value")
	f.env.SetOpSignedIn(t, true)
	if err := f.run(f.app.syncCommand(), "pagerduty-api-key"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	err := f.run(f.app.statusCommand())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	out := f.out.String()
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "LAST SYNC") {
		t.Fatalf("missing header:\n%s", out)
	}
	rows := strings.Split(out[strings.Index(out, "NAME"):], "\n")
	aws := rowWith(t, rows, "work/aws-access-key")
	if !strings.Contains(aws, "never") {
		t.Fatalf("unsynced row = %q", aws)
	}
	pd := rowWith(t, rows, "pagerduty-api-key")
	if strings.Contains(pd, "never") {
		t.Fatalf("synced row = %q", pd)
	}
	if !strings.Contains(pd, "PAGERDUTY_API_KEY") {
		t.Fatalf("synced row missing env: %q", pd)
	}
}

func TestStatusJSON(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	if err := f.run(f.app.statusCommand(), "--json"); err != nil {
		t.Fatalf("status --json: %v", err)
	}
	if !strings.Contains(f.out.String(), `"pagerduty-api-key"`) {
		t.Fatalf("output:\n%s", f.out.String())
	}
}

func rowWith(t *testing.T, rows []string, name string) string {
	t.Helper()
	for _, row := range rows {
		if strings.Contains(row, name) {
			return row
		}
	}
	t.Fatalf("no row with %s in %v", name, rows)
	return ""
}

func TestFormatAge(t *testing.T) {
	now := time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"never": {},
		"30s":   now.Add(-30 * time.Second),
		"5m":    now.Add(-5 * time.Minute),
		"2h":    now.Add(-2 * time.Hour),
		"3d":    now.Add(-72 * time.Hour),
	}
	for want, at := range cases {
		if got := formatAge(at, now); got != want {
			t.Errorf("formatAge(%v) = %q, want %q", at, got, want)
		}
	}
}
