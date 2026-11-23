package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
)

type auditOptions struct {
	since  string
	json   bool
	verify bool
}

func (a *app) auditCommand() *cobra.Command {
	var options auditOptions
	cmd := a.command("audit [NAME]", func(cmd *cobra.Command, args []string) error {
		if options.verify {
			return a.runAuditVerify()
		}
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		return a.runAudit(name, options)
	})
	cmd.Short = "Show the audit log"
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Flags().StringVar(&options.since, "since", "", "only events newer than this Go duration, e.g. 24h")
	cmd.Flags().BoolVar(&options.json, "json", false, "print one JSON event per line")
	cmd.Flags().BoolVar(&options.verify, "verify", false, "check the hash chain")
	return cmd
}

func (a *app) runAuditVerify() error {
	count, err := audit.Verify(a.paths.Audit())
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "OK, %d events, chain intact\n", count)
	return nil
}

func (a *app) runAudit(name string, options auditOptions) error {
	events, err := a.auditEvents(name, options.since)
	if err != nil {
		return err
	}
	if options.json {
		return a.printAuditJSON(events)
	}
	if len(events) == 0 {
		fmt.Fprintln(a.out, "No audit events")
		return nil
	}
	fmt.Fprintln(a.out, auditLine("TIME", audit.Event{Entry: "ENTRY", Action: "ACTION", Via: "VIA", User: "USER"}, "WHAT"))
	for _, event := range events {
		fmt.Fprintln(a.out, auditLine(event.Time.Local().Format("2006-01-02 15:04:05"), event, event.What()))
	}
	fmt.Fprintf(a.out, "%d events · %s\n", len(events), a.paths.Audit())
	return nil
}

// auditEvents returns the events for name (all when empty) newer than
// since (all when empty), newest first.
func (a *app) auditEvents(name, since string) ([]audit.Event, error) {
	var cutoff time.Time
	if since != "" {
		duration, err := time.ParseDuration(since)
		if err != nil {
			return nil, fmt.Errorf("invalid --since %q: %w", since, err)
		}
		cutoff = a.now().Add(-duration)
	}
	events, err := audit.Read(a.paths.Audit())
	if err != nil {
		return nil, err
	}
	events = slices.DeleteFunc(events, func(event audit.Event) bool {
		return (name != "" && event.Entry != name) || event.Time.Before(cutoff)
	})
	slices.Reverse(events)
	return events, nil
}

func (a *app) printAuditJSON(events []audit.Event) error {
	encoder := json.NewEncoder(a.out)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	return nil
}

// auditLine renders one row: TIME  ENTRY  ACTION  VIA  USER  WHAT.
func auditLine(timestamp string, event audit.Event, what string) string {
	return fmt.Sprintf("%-20s %-11s %-10s %-9s %-9s %s",
		timestamp, event.Entry, event.Action, event.Via, event.User, what)
}
