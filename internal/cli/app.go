package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/audit"
	"github.com/atavares-klaviyo/pass-vault/internal/execx"
	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/opcli"
	"github.com/atavares-klaviyo/pass-vault/internal/paths"
	"github.com/atavares-klaviyo/pass-vault/internal/store"
)

// app carries the shared dependencies of the data-plane commands.
type app struct {
	paths  paths.Paths
	runner execx.Runner
	in     io.Reader
	out    io.Writer
	errOut io.Writer
	// now is the clock; tests fix it to keep output deterministic.
	now func() time.Time
	// via is the subcommand name that audit events record.
	via string
}

func newApp() *app {
	return &app{
		paths:  paths.FromEnv(),
		runner: execx.OS{},
		in:     os.Stdin,
		out:    os.Stdout,
		errOut: os.Stderr,
		now:    time.Now,
	}
}

func (a *app) store() *store.Store {
	return &store.Store{Runner: a.runner, Dir: a.paths.StoreDir, Recorder: a.recorder()}
}

// recorder logs reads and writes of the entries the mapping file marks audited.
func (a *app) recorder() *audit.Recorder {
	return &audit.Recorder{
		Path: a.paths.Audit(),
		Via:  a.via,
		Now:  a.now,
		Audited: func(name string) bool {
			m, err := mapping.Load(a.paths.Mapping())
			return (err == nil && m.IsAudited(name)) || audit.WasAudited(a.paths.AuditFlags(), name)
		},
	}
}

func (a *app) opClient(m mapping.Mapping) *opcli.Client {
	return &opcli.Client{Runner: a.runner, Account: m.OpAccount}
}

func (a *app) loadMapping() (mapping.Mapping, error) {
	m, err := mapping.Load(a.paths.Mapping())
	if errors.Is(err, os.ErrNotExist) {
		return mapping.Mapping{}, fmt.Errorf("no mapping file at %s; run pvault mapping init", a.paths.Mapping())
	}
	if err != nil {
		return m, err
	}
	// A failed audit-flag sync must not fail the command: env runs at shell start.
	if err := audit.SyncFlags(a.recorder(), a.paths.AuditFlags(), m.AuditedNames()); err != nil {
		fmt.Fprintf(a.errOut, "pvault: audit log: %v\n", err)
	}
	return m, nil
}

func registerDataCommands(root *cobra.Command, a *app) {
	root.AddCommand(
		a.syncCommand(),
		a.checkCommand(),
		a.statusCommand(),
		a.envCommand(),
		a.runCommand(),
		a.getCommand(),
		a.historyCommand(),
		a.auditCommand(),
		a.rollbackCommand(),
		a.mappingCommand(),
	)
}

// command builds a cobra command with the app's streams wired in.
func (a *app) command(use string, run func(cmd *cobra.Command, args []string) error) *cobra.Command {
	cmd := &cobra.Command{Use: use, SilenceUsage: true, SilenceErrors: true, RunE: run}
	cmd.SetIn(a.in)
	cmd.SetOut(a.out)
	cmd.SetErr(a.errOut)
	return cmd
}
