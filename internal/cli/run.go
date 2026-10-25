package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/execx"
	"github.com/atavares-klaviyo/pass-vault/internal/shellenv"
)

func (a *app) runCommand() *cobra.Command {
	var entries []string
	cmd := a.command("run [--entry NAME]… -- CMD ARGS…", func(cmd *cobra.Command, args []string) error {
		return a.runWithEnv(cmd, entries, args)
	})
	cmd.Short = "Run CMD with the named entries in its environment"
	cmd.Flags().StringArrayVar(&entries, "entry", nil, "pass entry to expose; default is every entry with an env var")
	return cmd
}

func (a *app) runWithEnv(cmd *cobra.Command, entries, argv []string) error {
	if len(argv) == 0 {
		return errors.New("usage: pvault run [--entry NAME]… -- CMD ARGS…")
	}
	m, err := a.loadMapping()
	if err != nil {
		return err
	}
	vault := a.store()
	pairs, err := shellenv.RunEnv(cmd.Context(), m, shellenv.RunRequest{Names: entries, Read: vault.Show})
	if err != nil {
		return err
	}
	return a.runChild(cmd, argv, pairs)
}

func (a *app) runChild(cmd *cobra.Command, argv, pairs []string) error {
	_, err := a.runner.Run(cmd.Context(), execx.Cmd{Name: argv[0], Args: argv[1:], Env: pairs, Terminal: true})
	var exitErr *execx.ExitError
	if errors.As(err, &exitErr) {
		return ExitCodeError{Code: exitErr.Code, Message: fmt.Sprintf("%s exited %d", argv[0], exitErr.Code)}
	}
	return err
}
