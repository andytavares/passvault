package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/block"
	"github.com/atavares-klaviyo/pass-vault/internal/execx"
	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/opcli"
	"github.com/atavares-klaviyo/pass-vault/internal/setup"
)

// check is one doctor outcome: one of ✓ (ok), ! (warning) or ✕ (failure).
type check struct {
	glyph  string
	name   string
	detail string
}

func (a *app) doctorCommand() *cobra.Command {
	cmd := a.command("doctor", func(cmd *cobra.Command, _ []string) error {
		return a.runDoctor(cmd.Context())
	})
	cmd.Short = "Check the tools, store, mapping and shell setup"
	return cmd
}

func (a *app) runDoctor(ctx context.Context) error {
	checks := a.doctorChecks(ctx)
	for _, c := range checks {
		fmt.Fprintf(a.out, "%s %s  %s\n", c.glyph, c.name, c.detail)
	}
	for _, c := range checks {
		if c.glyph == "✕" {
			return ExitCodeError{Code: 1, Message: "doctor found problems"}
		}
	}
	return nil
}

// toolProbe is one versioned tool doctor checks: name, argv and a substring
// the first output line must hold (empty means any output is fine).
type toolProbe struct {
	name string
	args []string
	want string
}

func (a *app) doctorChecks(ctx context.Context) []check {
	var checks []check
	probes := []toolProbe{
		{"pass", []string{"version"}, "v1."},
		{"gpg", []string{"--version"}, "gpg (GnuPG)"},
		{"op", []string{"--version"}, ""},
	}
	for _, probe := range probes {
		checks = append(checks, a.toolCheck(ctx, probe))
	}
	checks = append(checks, a.opSignInCheck(ctx))
	checks = append(checks, a.pinentryCheck())
	checks = append(checks, a.storeChecks(ctx)...)
	checks = append(checks, a.mappingChecks()...)
	return append(checks, a.zshrcChecks()...)
}

// toolCheck runs the probe and requires want in some output line; the
// matching line is the detail, so `pass version`'s banner is skipped.
func (a *app) toolCheck(ctx context.Context, probe toolProbe) check {
	result, err := a.runner.Run(ctx, execx.Cmd{Name: probe.name, Args: probe.args})
	if err != nil {
		return check{"✕", probe.name + " version", "missing: " + err.Error()}
	}
	lines := strings.Split(strings.TrimSpace(string(result.Stdout)), "\n")
	for _, line := range lines {
		if probe.want == "" || strings.Contains(line, probe.want) {
			return check{"✓", probe.name + " version", strings.TrimSpace(line)}
		}
	}
	return check{"✕", probe.name + " version", "unexpected: " + lines[0]}
}

func (a *app) opSignInCheck(ctx context.Context) check {
	client := &opcli.Client{Runner: a.runner}
	if err := client.SignedIn(ctx); err != nil {
		return check{"!", "op sign-in", "not signed in: enable Settings > Developer > Integrate with 1Password CLI"}
	}
	return check{"✓", "op sign-in", "signed in"}
}

// pinentryCheck wants a pinentry-program line inside or outside the block.
func (a *app) pinentryCheck() check {
	content, err := os.ReadFile(a.paths.GpgAgentConf())
	if err != nil {
		return check{"!", "pinentry-program", "not configured in gpg-agent.conf"}
	}
	body, _, _ := block.Remove(string(content))
	if strings.Contains(body, "pinentry-program") || block.Has(string(content)) {
		return check{"✓", "pinentry-program", "configured in gpg-agent.conf"}
	}
	return check{"!", "pinentry-program", "not configured in gpg-agent.conf"}
}

func (a *app) storeChecks(ctx context.Context) []check {
	ids, err := os.ReadFile(filepath.Join(a.paths.StoreDir, ".gpg-id"))
	if err != nil {
		return []check{{"✕", "pass store", "missing: " + a.paths.StoreDir}}
	}
	key := strings.Fields(string(ids))[0]
	_, err = a.runner.Run(ctx, execx.Cmd{Name: "gpg", Args: []string{"--list-secret-keys", key}})
	if err != nil {
		return []check{{"✕", "pass store", "no secret key for .gpg-id " + key}}
	}
	checks := []check{{"✓", "pass store", ".gpg-id matches a secret key"}}
	if _, err := os.Stat(filepath.Join(a.paths.StoreDir, ".git")); err != nil {
		return append(checks, check{"!", "store git", "not a git repo: no sync history or rollback"})
	}
	return append(checks, check{"✓", "store git", "git repo present"})
}

func (a *app) mappingChecks() []check {
	m, err := mapping.Load(a.paths.Mapping())
	if err != nil {
		if os.IsNotExist(err) {
			return []check{{"!", "mapping.toml", "missing: run pvault mapping init"}}
		}
		return []check{{"✕", "mapping.toml", err.Error()}}
	}
	checks := []check{{"✓", "mapping.toml", fmt.Sprintf("%d entries, valid", len(m.Secrets))}}
	for _, secret := range m.Secrets {
		if secret.Scope == mapping.ScopeGlobal && secret.Env != "" {
			checks = append(checks, check{"!", "global scope", secret.Env + " exported to every child process"})
		}
	}
	return checks
}

func (a *app) zshrcChecks() []check {
	var checks []check
	for _, finding := range setup.ScanZshrcForPass(a.paths.Zshrc()) {
		checks = append(checks, check{"!", "zshrc", finding})
	}
	return checks
}
