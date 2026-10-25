package cli

import (
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

func TestSummaryLine(t *testing.T) {
	tagged := syncer.Report{Tag: "sync-20261025T130512", Added: 1, Updated: 1, Unchanged: 2, Orphans: 1}
	untouched := syncer.Report{Unchanged: 3}
	cases := map[string]struct {
		report syncer.Report
		want   string
	}{
		"tagged run":      {tagged, "Tagged sync-20261025T130512 · 2 changed · 2 unchanged · 1 orphan"},
		"nothing changed": {untouched, "No changes · 0 changed · 3 unchanged · 0 orphan"},
		"dry run no tag":  {syncer.Report{Added: 2}, "No changes tagged · 2 changed · 0 unchanged · 0 orphan"},
	}
	for name, tc := range cases {
		if got := summaryLine(tc.report); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}
