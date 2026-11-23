package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
)

func (a *app) getCommand() *cobra.Command {
	cmd := a.command("get NAME", func(cmd *cobra.Command, args []string) error {
		return a.runGet(cmd, args[0])
	})
	cmd.Short = "Print one entry's value"
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

// runGet writes the exact bytes of name to stdout, with no newline added.
func (a *app) runGet(cmd *cobra.Command, name string) error {
	vault := a.store()
	if !vault.Exists(name) {
		return fmt.Errorf("unknown pass entry: %s", name)
	}
	value, err := vault.Show(cmd.Context(), name, audit.ActionUse)
	if err != nil {
		return err
	}
	_, err = a.out.Write(value)
	return err
}
