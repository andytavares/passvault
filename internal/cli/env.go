package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/shellenv"
)

func (a *app) envCommand() *cobra.Command {
	cmd := a.command("env [--zsh]", func(cmd *cobra.Command, _ []string) error {
		a.runEnv(cmd)
		return nil
	})
	cmd.Short = "Print the zsh exports and command wrappers"
	cmd.Args = cobra.NoArgs
	// --zsh is the only shell; the flag stays for the zshrc snippet.
	cmd.Flags().Bool("zsh", false, "print a zsh script (the only shell)")
	return cmd
}

// runEnv never fails: a broken shell start is worse than a missing secret.
func (a *app) runEnv(cmd *cobra.Command) {
	m, err := a.loadMapping()
	if err != nil {
		return
	}
	vault := a.store()
	script, warnings := shellenv.RenderScript(cmd.Context(), m, vault.Show)
	fmt.Fprint(a.out, script)
	for _, warning := range warnings {
		fmt.Fprintf(a.errOut, "pvault: skipped %s: %s\n", warning.Pass, warning.Reason)
	}
}
