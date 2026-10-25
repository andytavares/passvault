package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// rollbackFlags are the flags of `pvault rollback`.
type rollbackFlags struct {
	entry string
	yes   bool
}

func (a *app) rollbackCommand() *cobra.Command {
	flags := rollbackFlags{}
	cmd := a.command("rollback REV", func(cmd *cobra.Command, args []string) error {
		return a.runRollback(cmd, args[0], flags)
	})
	cmd.Short = "Restore the store, or one entry, to REV with a new commit"
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().StringVar(&flags.entry, "entry", "", "restore only this pass entry")
	cmd.Flags().BoolVar(&flags.yes, "yes", false, "do not ask for confirmation")
	return cmd
}

func (a *app) runRollback(cmd *cobra.Command, revision string, flags rollbackFlags) error {
	vault := a.store()
	restored, removed, err := vault.RollbackPlan(cmd.Context(), revision, flags.entry)
	if err != nil {
		return err
	}
	if len(restored) == 0 && len(removed) == 0 {
		fmt.Fprintln(a.out, "Nothing to roll back")
		return nil
	}
	fmt.Fprintf(a.out, "Restores %d entries, removes %d (%s)\n", len(restored), len(removed), strings.Join(removed, ", "))
	if !flags.yes && !a.confirm("Continue? [y/N] ") {
		return nil
	}
	if _, err := vault.Rollback(cmd.Context(), revision, flags.entry); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Committed pvault rollback to %s · undo with pvault rollback HEAD~1\n", revision)
	return nil
}

// confirm reads one line and reports whether it was y or yes.
func (a *app) confirm(question string) bool {
	fmt.Fprint(a.out, question)
	line, err := bufio.NewReader(a.in).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
