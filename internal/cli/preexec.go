package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/shellenv"
)

func (a *app) preexecCommand() *cobra.Command {
	cmd := a.command("preexec -- CMDLINE", func(_ *cobra.Command, args []string) error {
		return a.runPreexec(strings.Join(args, " "))
	})
	cmd.Short = "Log a shell command that starts with an audited global secret"
	cmd.Hidden = true
	return cmd
}

// runPreexec records cmdline as inherited by each audited global secret set
// in this shell; an export that failed at shell start is not logged. It
// runs before every shell command, so it never reads the pass store and
// ignores a missing or broken mapping.
func (a *app) runPreexec(cmdline string) error {
	if cmdline == "" {
		return nil
	}
	m, err := mapping.Load(a.paths.Mapping())
	if err != nil {
		return nil
	}
	recorder := a.recorder()
	for _, secret := range m.Secrets {
		if shellenv.IsAuditedGlobal(secret) && os.Getenv(secret.Env) != "" {
			if err := recorder.Record(secret.Pass, audit.ActionInherit, cmdline); err != nil {
				return err
			}
		}
	}
	return nil
}
