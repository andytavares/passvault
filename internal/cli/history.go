package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/store"
)

func (a *app) historyCommand() *cobra.Command {
	var limit int
	cmd := a.command("history [NAME]", func(cmd *cobra.Command, args []string) error {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		return a.runHistory(cmd, name, limit)
	})
	cmd.Short = "Show store commits and sync tags"
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum commits to list")
	return cmd
}

func (a *app) runHistory(cmd *cobra.Command, name string, limit int) error {
	vault := a.store()
	if name == "" {
		if err := a.printSyncTags(cmd, vault); err != nil {
			return err
		}
	}
	revisions, err := vault.History(cmd.Context(), name, limit)
	if err != nil {
		return err
	}
	for _, revision := range revisions {
		fmt.Fprintln(a.out, historyLine(revision))
	}
	return nil
}

func (a *app) printSyncTags(cmd *cobra.Command, vault *store.Store) error {
	tags, err := vault.SyncTags(cmd.Context())
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Sync tags: %s\n", strings.Join(tags, ", "))
	return nil
}

// historyLine renders one commit: SHORT  date time  subject  [tags].
func historyLine(revision store.Revision) string {
	line := fmt.Sprintf("%s  %s  %s",
		revision.Short, revision.Time.Local().Format("2006-01-02 15:04"), revision.Subject)
	if len(revision.Tags) > 0 {
		line += "  [" + strings.Join(revision.Tags, ", ") + "]"
	}
	return line
}
