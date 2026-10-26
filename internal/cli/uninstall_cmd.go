package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/setup"
)

type uninstallFlags struct {
	purgeVault bool
	purgeKey   bool
	keepConfig bool
	dryRun     bool
	yes        bool
}

func (a *app) uninstallCommand() *cobra.Command {
	flags := uninstallFlags{}
	cmd := a.command("uninstall", func(cmd *cobra.Command, _ []string) error {
		return a.runUninstall(cmd.Context(), flags)
	})
	cmd.Short = "Remove what pvault setup installed"
	cmd.Flags().BoolVar(&flags.purgeVault, "purge-vault", false, "also remove the pass store")
	cmd.Flags().BoolVar(&flags.purgeKey, "purge-key", false, "also delete the GPG key pvault created")
	cmd.Flags().BoolVar(&flags.keepConfig, "keep-config", false, "keep the pvault config directory")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "print the plan without removing")
	cmd.Flags().BoolVar(&flags.yes, "yes", false, "skip the confirmation prompt")
	return cmd
}

func (a *app) runUninstall(ctx context.Context, flags uninstallFlags) error {
	installer, err := a.newInstaller()
	if err != nil {
		return err
	}
	opts := setup.UninstallOptions{
		PurgeVault: flags.purgeVault, PurgeKey: flags.purgeKey,
		KeepConfig: flags.keepConfig, DryRun: flags.dryRun,
	}
	plan, err := installer.Plan(ctx, opts)
	if err != nil {
		return err
	}
	a.printUninstallPlan(plan)
	if flags.dryRun {
		return nil
	}
	// The installer's prompter is reused so its buffered reader keeps any
	// unread stdin for the vault path confirmation inside Uninstall.
	if !flags.yes && !installer.Prompt.Confirm("Continue?") {
		return nil
	}
	return installer.Uninstall(ctx, opts)
}

// printUninstallPlan renders the plan in the shape of the design mockup.
func (a *app) printUninstallPlan(plan setup.UninstallPlan) {
	fmt.Fprintln(a.out, "Will remove")
	for _, item := range plan.Remove {
		fmt.Fprintf(a.out, "  - %s\n", item)
	}
	fmt.Fprintln(a.out, "Will keep")
	for _, keep := range plan.Keep {
		fmt.Fprintf(a.out, "  · %s  %s\n", keep.Item, keep.Reason)
	}
}
