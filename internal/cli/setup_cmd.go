package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/setup"
)

// registerSetupCommands wires setup, upgrade, uninstall and doctor.
func registerSetupCommands(root *cobra.Command, a *app) {
	root.AddCommand(a.setupCommand(), a.upgradeCommand(), a.uninstallCommand(), a.doctorCommand())
}

// cliReporter prints one glyph line per setup step.
type cliReporter struct{ out io.Writer }

func (r *cliReporter) Report(status setup.Status, name, detail string) {
	fmt.Fprintf(r.out, "  %s %s  %s\n", status.Glyph(), name, detail)
}

// terminalPrompter asks questions on in and prints them on out.
type terminalPrompter struct {
	in  *bufio.Reader
	out io.Writer
}

func newTerminalPrompter(in io.Reader, out io.Writer) *terminalPrompter {
	return &terminalPrompter{in: bufio.NewReader(in), out: out}
}

func (p *terminalPrompter) Confirm(question string) bool {
	fmt.Fprintf(p.out, "%s [y/N] ", question)
	answer, _ := p.in.ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(answer), "y")
}

func (p *terminalPrompter) Ask(question, fallback string) string {
	fmt.Fprintf(p.out, "%s [%s]: ", question, fallback)
	answer, _ := p.in.ReadString('\n')
	if trimmed := strings.TrimSpace(answer); trimmed != "" {
		return trimmed
	}
	return fallback
}

// newInstaller builds the setup.Installer the setup-family commands share.
func (a *app) newInstaller() (*setup.Installer, error) {
	manifest, err := setup.LoadManifest(a.paths.Manifest())
	if err != nil {
		return nil, err
	}
	return &setup.Installer{
		Runner:   a.runner,
		Paths:    a.paths,
		Manifest: manifest,
		Prompt:   newTerminalPrompter(a.in, a.out),
		Now:      a.now,
		Out:      &cliReporter{out: a.out},
	}, nil
}

type setupFlags struct {
	dryRun bool
	source string
}

func (a *app) setupCommand() *cobra.Command {
	flags := setupFlags{}
	cmd := a.command("setup", func(cmd *cobra.Command, _ []string) error {
		return a.runSetup(cmd.Context(), flags)
	})
	cmd.Short = "Install the tools, key, store and shell block pvault needs"
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "print the plan without writing")
	cmd.Flags().StringVar(&flags.source, "source", "", "source clone install.sh built from")
	return cmd
}

func (a *app) runSetup(ctx context.Context, flags setupFlags) error {
	installer, err := a.newInstaller()
	if err != nil {
		return err
	}
	installer.DryRun = flags.dryRun
	installer.SourceDir = flags.source
	fmt.Fprintln(a.out, "pvault setup")
	if err := installer.Setup(ctx); err != nil {
		return ExitCodeError{Code: 1, Message: err.Error()}
	}
	fmt.Fprintf(a.out, "Manifest: %s\n", a.paths.Manifest())
	return nil
}
