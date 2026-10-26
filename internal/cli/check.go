package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

func (a *app) checkCommand() *cobra.Command {
	cmd := a.command("check [NAME…]", func(cmd *cobra.Command, args []string) error {
		return a.runCheck(cmd.Context(), args)
	})
	cmd.Short = "Compare 1Password and pass without writing"
	return cmd
}

func (a *app) runCheck(ctx context.Context, names []string) error {
	m, err := a.loadMapping()
	if err != nil {
		return err
	}
	client := a.opClient(m)
	if err := client.SignedIn(ctx); err != nil {
		return err
	}
	sync, err := a.newSyncer(client, true)
	if err != nil {
		return err
	}
	entries, err := sync.Check(ctx, m, names)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		fmt.Fprintln(a.out, reportLine(entry))
	}
	return checkExitCode(entries)
}

// checkExitCode is 1 when any entry is not unchanged.
func checkExitCode(entries []syncer.Entry) error {
	dirty := 0
	for _, entry := range entries {
		if entry.Action != syncer.ActionUnchanged {
			dirty++
		}
	}
	if dirty > 0 {
		return ExitCodeError{Code: 1, Message: fmt.Sprintf("%d entries not in sync", dirty)}
	}
	return nil
}
