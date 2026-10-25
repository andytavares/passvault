package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

func (a *app) statusCommand() *cobra.Command {
	var asJSON bool
	cmd := a.command("status", func(cmd *cobra.Command, _ []string) error {
		return a.runStatus(asJSON)
	})
	cmd.Short = "List entries with their last sync time"
	cmd.Args = cobra.NoArgs
	cmd.Flags().BoolVar(&asJSON, "json", false, "print rows as JSON")
	return cmd
}

func (a *app) runStatus(asJSON bool) error {
	m, err := a.loadMapping()
	if err != nil {
		return err
	}
	state, err := syncer.LoadState(a.paths.State())
	if err != nil {
		return err
	}
	names, err := a.store().List()
	if err != nil {
		return err
	}
	rows := syncer.Status(m, state, names)
	if asJSON {
		encoder := json.NewEncoder(a.out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(rows)
	}
	printStatusTable(a.out, rows, a.now())
	return nil
}

// printStatusTable writes the NAME KIND ENV SCOPE LAST SYNC table.
func printStatusTable(out io.Writer, rows []syncer.StatusRow, now time.Time) {
	fmt.Fprintf(out, "%-28s %-14s %-18s %-8s %s\n", "NAME", "KIND", "ENV", "SCOPE", "LAST SYNC")
	for _, row := range rows {
		fmt.Fprintf(out, "%-28s %-14s %-18s %-8s %s\n",
			row.Name, row.Kind, dash(row.Env), dash(string(row.Scope)), formatAge(row.SyncedAt, now))
	}
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// formatAge renders t relative to now as a short duration, or "never".
func formatAge(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	age := now.Sub(t)
	switch {
	case age < 0:
		age = 0
	}
	switch {
	case age < time.Minute:
		return fmt.Sprintf("%ds", int(age.Seconds()))
	case age < time.Hour:
		return fmt.Sprintf("%dm", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age.Hours()))
	default:
		return fmt.Sprintf("%dd", int(age.Hours()/24))
	}
}
