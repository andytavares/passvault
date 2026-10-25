package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/execx"
	"github.com/atavares-klaviyo/pass-vault/internal/setup"
)

func (a *app) upgradeCommand() *cobra.Command {
	var noTools bool
	cmd := a.command("upgrade", func(cmd *cobra.Command, _ []string) error {
		return a.runUpgrade(cmd.Context(), noTools)
	})
	cmd.Short = "Pull the source, rebuild, swap the binary and run migrations"
	cmd.Flags().BoolVar(&noTools, "no-tools", false, "upgrade only pvault, not the brew toolset")
	return cmd
}

// runUpgrade rebuilds from the source clone, then runs the new binary's
// setup as a child so migrations run with the new code, not this process.
func (a *app) runUpgrade(ctx context.Context, noTools bool) error {
	installer, err := a.newInstaller()
	if err != nil {
		return err
	}
	if err := installer.Upgrade(ctx, setup.UpgradeOptions{NoTools: noTools}); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Upgraded %s\n", a.paths.Binary())
	fmt.Fprintln(a.out, "This running process is still the old version.")
	_, err = a.runner.Run(ctx, execx.Cmd{Name: a.paths.Binary(), Args: []string{"setup"}, Terminal: true})
	return err
}
