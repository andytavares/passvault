package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

// syncFlags are the flags shared by `pvault sync` and `pvault check`.
type syncFlags struct {
	only          []string
	prune         bool
	skipConflicts bool
	dryRun        bool
}

func (a *app) syncCommand() *cobra.Command {
	flags := syncFlags{}
	cmd := a.command("sync [NAME…]", func(cmd *cobra.Command, args []string) error {
		flags.only = args
		return a.runSync(cmd.Context(), flags)
	})
	cmd.Short = "Sync secrets from 1Password into pass"
	cmd.Flags().BoolVar(&flags.prune, "prune", false, "delete entries no longer in the mapping")
	cmd.Flags().BoolVar(&flags.skipConflicts, "skip-conflicts", false, "never overwrite; exit 1 when an entry differs")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "print the plan without writing")
	return cmd
}

func (a *app) runSync(ctx context.Context, flags syncFlags) error {
	m, err := a.loadMapping()
	if err != nil {
		return err
	}
	client := a.opClient(m)
	if err := client.SignedIn(ctx); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Reading %d entries from 1Password (Touch ID may prompt)\n", len(m.Secrets))
	sync, err := a.newSyncer(client, flags.skipConflicts)
	if err != nil {
		return err
	}
	report, err := sync.Sync(ctx, m, syncer.SyncOptions{
		Only: flags.only, Prune: flags.prune,
		SkipConflicts: flags.skipConflicts, DryRun: flags.dryRun,
	})
	if err != nil {
		return err
	}
	return a.finishSync(report, *sync.State, flags)
}

// newSyncer loads state and builds the Syncer with the CLI decider.
func (a *app) newSyncer(client syncer.Source, skipConflicts bool) (*syncer.Syncer, error) {
	state, err := syncer.LoadState(a.paths.State())
	if err != nil {
		return nil, err
	}
	sync := &syncer.Syncer{Source: client, Vault: a.store(), State: &state, Now: a.now}
	if !skipConflicts {
		sync.Decider = &cliDecider{in: a.in, out: a.out, errOut: a.errOut}
	}
	return sync, nil
}

// finishSync prints the report, saves state and maps it to an exit code.
func (a *app) finishSync(report syncer.Report, state syncer.State, flags syncFlags) error {
	printReport(a.out, report)
	fmt.Fprintln(a.out, summaryLine(report))
	if !flags.dryRun {
		if err := syncer.SaveState(a.paths.State(), state); err != nil {
			return err
		}
	}
	return syncExitCode(report, flags.skipConflicts)
}

// summaryLine ends a sync report. A run that changed nothing has no tag, and
// a dry run changes entries without tagging.
func summaryLine(report syncer.Report) string {
	changed := report.Added + report.Updated
	head := "Tagged " + report.Tag
	if report.Tag == "" {
		head = "No changes"
		if changed > 0 {
			head = "No changes tagged"
		}
	}
	return fmt.Sprintf("%s · %d changed · %d unchanged · %d orphan", head, changed, report.Unchanged, report.Orphans)
}

func syncExitCode(report syncer.Report, skipConflicts bool) error {
	if report.Failed > 0 {
		return ExitCodeError{Code: 2, Message: fmt.Sprintf("%d entries failed", report.Failed)}
	}
	if skipConflicts && report.Differs() > 0 {
		return ExitCodeError{Code: 1, Message: fmt.Sprintf("%d entries differ", report.Differs())}
	}
	return nil
}

// printReport writes one glyph line per entry.
func printReport(out io.Writer, report syncer.Report) {
	for _, entry := range report.Entries {
		fmt.Fprintln(out, reportLine(entry))
	}
}

// reportLine renders one entry of a sync or check report. A post-sync
// differs line means the conflict was skipped; a check differs line means
// the entry drifts.
func reportLine(entry syncer.Entry) string {
	name := entry.Secret.Pass
	switch entry.Action {
	case syncer.ActionUnchanged:
		return fmt.Sprintf("  = %s  unchanged", name)
	case syncer.ActionAdd:
		return fmt.Sprintf("  + %s  added", name)
	case syncer.ActionDiffers:
		return fmt.Sprintf("  ~ %s  differs  1Password %d chars · pass %d chars", name, entry.SourceLen, entry.VaultLen)
	case syncer.ActionOrphan:
		return fmt.Sprintf("  ? %s  orphan, no longer in mapping · run pvault sync --prune to delete", name)
	case syncer.ActionFailed:
		return fmt.Sprintf("  ✕ %s  failed: %v", name, entry.Err)
	}
	return "  · " + name
}
