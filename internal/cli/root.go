// Package cli defines the pvault cobra commands.
package cli

import (
	"github.com/spf13/cobra"
)

// Version is set at build time with -ldflags "-X .../internal/cli.Version=…".
var Version = "dev"

// NewRoot builds the pvault command tree.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "pvault",
		Short:         "Sync 1Password secrets into pass and manage the vault",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVersionCommand())
	application := newApp()
	registerDataCommands(root, application)
	registerSetupCommands(root, application)
	registerTUICommand(root, application)
	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the pvault version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("pvault " + Version)
			return nil
		},
	}
}
