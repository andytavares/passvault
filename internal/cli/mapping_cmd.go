package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

func (a *app) mappingCommand() *cobra.Command {
	cmd := a.command("mapping", nil)
	cmd.Short = "Manage the mapping file"
	cmd.AddCommand(a.mappingInitCommand(), a.mappingPathCommand(), a.mappingValidateCommand(), a.mappingAddCommand())
	return cmd
}

func (a *app) mappingInitCommand() *cobra.Command {
	cmd := a.command("init", func(_ *cobra.Command, _ []string) error {
		created, err := mapping.WriteExample(a.paths.Mapping())
		if err != nil {
			return err
		}
		if created {
			fmt.Fprintf(a.out, "Created %s\n", a.paths.Mapping())
		} else {
			fmt.Fprintf(a.out, "%s already exists\n", a.paths.Mapping())
		}
		return nil
	})
	cmd.Short = "Write the example mapping file"
	cmd.Args = cobra.NoArgs
	return cmd
}

func (a *app) mappingPathCommand() *cobra.Command {
	cmd := a.command("path", func(_ *cobra.Command, _ []string) error {
		fmt.Fprintln(a.out, a.paths.Mapping())
		return nil
	})
	cmd.Short = "Print the mapping file path"
	cmd.Args = cobra.NoArgs
	return cmd
}

func (a *app) mappingValidateCommand() *cobra.Command {
	cmd := a.command("validate", func(_ *cobra.Command, _ []string) error {
		m, err := mapping.Load(a.paths.Mapping())
		if err != nil {
			for _, line := range strings.Split(err.Error(), "\n") {
				fmt.Fprintln(a.out, line)
			}
			return ExitCodeError{Code: 1, Message: "mapping is invalid"}
		}
		fmt.Fprintf(a.out, "OK, %d entries\n", len(m.Secrets))
		return nil
	})
	cmd.Short = "Check the mapping file"
	cmd.Args = cobra.NoArgs
	return cmd
}

// addFlags are the non-interactive flags of `pvault mapping add`.
type addFlags struct {
	pass     string
	ref      string
	env      string
	scope    string
	commands []string
}

func (a *app) mappingAddCommand() *cobra.Command {
	flags := addFlags{}
	cmd := a.command("add", func(_ *cobra.Command, _ []string) error {
		return a.runMappingAdd(flags)
	})
	cmd.Short = "Append a secret to the mapping file"
	cmd.Args = cobra.NoArgs
	cmd.Flags().StringVar(&flags.pass, "pass", "", "path in the pass store, without .gpg")
	cmd.Flags().StringVar(&flags.ref, "ref", "", "op:// reference")
	cmd.Flags().StringVar(&flags.env, "env", "", "environment variable name")
	cmd.Flags().StringVar(&flags.scope, "scope", "", "global (default) | command | none")
	cmd.Flags().StringArrayVar(&flags.commands, "command", nil, "command that gets the variable (scope command)")
	_ = cmd.MarkFlagRequired("pass")
	_ = cmd.MarkFlagRequired("ref")
	return cmd
}

func (a *app) runMappingAdd(flags addFlags) error {
	if _, err := a.loadMapping(); err != nil {
		return err
	}
	secret := mapping.Secret{
		Pass: flags.pass, Ref: flags.ref, Env: flags.env,
		Scope: mapping.Scope(flags.scope), Commands: flags.commands,
	}
	if err := mapping.AppendSecret(a.paths.Mapping(), secret); err != nil {
		return err
	}
	if _, err := mapping.Load(a.paths.Mapping()); err != nil {
		return errors.Join(fmt.Errorf("appended but re-validation failed"), err)
	}
	fmt.Fprintf(a.out, "Added %s to %s\n", flags.pass, a.paths.Mapping())
	return nil
}
