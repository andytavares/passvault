package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/atavares-klaviyo/pass-vault/internal/block"
	"github.com/atavares-klaviyo/pass-vault/internal/execx"
	"github.com/atavares-klaviyo/pass-vault/internal/paths"
)

// Status is how a step ended.
type Status int

const (
	// Present: the step had nothing to do.
	Present Status = iota
	// Applied: the step changed something.
	Applied
	// Skipped: the step did not run.
	Skipped
	// Warning: the step found something the user should see.
	Warning
	// Failed: the step could not finish.
	Failed
)

var glyphs = map[Status]string{
	Present: "✓", Applied: "+", Skipped: "-", Warning: "!", Failed: "✕",
}

// Glyph renders the status marker.
func (s Status) Glyph() string { return glyphs[s] }

// Step is one idempotent setup step.
type Step struct {
	Name  string
	Check func(ctx context.Context) (done bool, detail string, err error)
	Apply func(ctx context.Context) (detail string, err error)
}

// Reporter receives one line per step outcome.
type Reporter interface {
	Report(status Status, name, detail string)
}

// Prompter asks the user questions.
type Prompter interface {
	Confirm(question string) bool
	Ask(question, fallback string) string
}

// Installer runs the setup steps against one machine.
type Installer struct {
	Runner   execx.Runner
	Paths    paths.Paths
	Manifest *Manifest
	Prompt   Prompter
	Now      func() time.Time
	DryRun   bool
	Out      Reporter
	// SourceDir is the clone install.sh built from; the manifest stores it.
	SourceDir string
	// GOOS and GenerateKey are fields so tests can replace them.
	GOOS        string
	GenerateKey func(ctx context.Context, userID string) error
}

func (in *Installer) now() time.Time {
	if in.Now != nil {
		return in.Now()
	}
	return time.Now()
}

func (in *Installer) goos() string {
	if in.GOOS != "" {
		return in.GOOS
	}
	return runtime.GOOS
}

// Setup runs every step in order, runs migrations and saves the manifest.
func (in *Installer) Setup(ctx context.Context) error {
	if err := in.RunMigrations(ctx, migrations, CurrentSchema); err != nil {
		return err
	}
	var failed error
	for _, step := range in.steps() {
		if err := in.runStep(ctx, step); err != nil && failed == nil {
			failed = err
		}
	}
	if !in.DryRun {
		if in.SourceDir != "" {
			in.Manifest.SourceDir = in.SourceDir
		}
		if in.Manifest.InstalledAt.IsZero() {
			in.Manifest.InstalledAt = in.now()
		}
		in.Manifest.UpdatedAt = in.now()
		if err := in.Manifest.Save(in.Paths.Manifest()); err != nil {
			return err
		}
	}
	return failed
}

func (in *Installer) runStep(ctx context.Context, step Step) error {
	done, detail, err := step.Check(ctx)
	switch {
	case err != nil:
		in.Out.Report(Failed, step.Name, err.Error())
		return err
	case done:
		in.Out.Report(Present, step.Name, detail)
		return nil
	case in.DryRun:
		in.Out.Report(Skipped, step.Name, "dry run: would apply")
		return nil
	}
	detail, err = step.Apply(ctx)
	if err != nil {
		in.Out.Report(Failed, step.Name, err.Error())
		return err
	}
	in.Out.Report(Applied, step.Name, detail)
	return nil
}

func (in *Installer) steps() []Step {
	return []Step{
		in.preflightStep(),
		in.packagesStep(),
		in.pinentryStep(),
		in.keyStep(),
		in.storeStep(),
		in.gitStep(),
		in.mappingStep(),
		in.zshrcStep(),
		in.opStep(),
	}
}

func (in *Installer) preflightStep() Step {
	return Step{
		Name: "darwin + brew",
		Check: func(ctx context.Context) (bool, string, error) {
			if in.goos() != "darwin" {
				return false, "", fmt.Errorf("pvault needs macOS, found %s", in.goos())
			}
			if _, err := exec.LookPath("brew"); err != nil {
				return false, "", errors.New("brew not on PATH: install it from https://brew.sh")
			}
			return true, "darwin, brew present", nil
		},
		Apply: func(context.Context) (string, error) { return "", nil },
	}
}

var toolPackages = []Package{
	{Name: "pass"}, {Name: "gnupg"}, {Name: "pinentry-mac"}, {Name: "1password-cli", Cask: true},
}

func (in *Installer) packagesStep() Step {
	return Step{
		Name:  "brew packages",
		Check: func(ctx context.Context) (bool, string, error) { return in.packagesDone(ctx) },
		Apply: func(ctx context.Context) (string, error) { return in.installPackages(ctx) },
	}
}

func (in *Installer) packagesDone(ctx context.Context) (bool, string, error) {
	for _, pkg := range toolPackages {
		if _, err := in.Runner.Run(ctx, listCmd(pkg)); err != nil {
			return false, "", nil
		}
	}
	return true, "all packages present", nil
}

func (in *Installer) installPackages(ctx context.Context) (string, error) {
	for _, pkg := range toolPackages {
		if err := in.installPackage(ctx, pkg); err != nil {
			return "", err
		}
	}
	if os.Getenv("PVAULT_INSTALLED_GO") == "1" {
		in.Manifest.Add(Package{Name: "go"})
	}
	return "packages installed", nil
}

func (in *Installer) installPackage(ctx context.Context, pkg Package) error {
	if _, err := in.Runner.Run(ctx, listCmd(pkg)); err == nil {
		detail := "present, not managed"
		if in.Manifest.HasPackage(pkg.Name) {
			detail = "managed"
		}
		in.Out.Report(Present, pkg.Name, detail)
		return nil
	}
	args := []string{"install"}
	if pkg.Cask {
		args = append(args, "--cask")
	}
	args = append(args, pkg.Name)
	if _, err := in.Runner.Run(ctx, execx.Cmd{Name: "brew", Args: args}); err != nil {
		return err
	}
	in.Manifest.Add(pkg)
	in.Out.Report(Applied, pkg.Name, "installed")
	return nil
}

func listCmd(pkg Package) execx.Cmd {
	args := []string{"list"}
	if pkg.Cask {
		args = append(args, "--cask")
	}
	return execx.Cmd{Name: "brew", Args: append(args, "--versions", pkg.Name)}
}

func (in *Installer) pinentryStep() Step {
	return Step{
		Name:  "gpg-agent.conf",
		Check: func(ctx context.Context) (bool, string, error) { return in.pinentryDone(ctx) },
		Apply: func(ctx context.Context) (string, error) { return in.writePinentry(ctx) },
	}
}

func (in *Installer) pinentryDone(ctx context.Context) (bool, string, error) {
	content, err := os.ReadFile(in.Paths.GpgAgentConf())
	if err != nil && !os.IsNotExist(err) {
		return false, "", err
	}
	if hasPinentryOutsideBlock(string(content)) {
		in.Out.Report(Warning, "gpg-agent.conf", "pinentry-program set outside the managed block; leaving it")
		return true, "user pinentry-program", nil
	}
	if block.Has(string(content)) {
		return true, "managed block present", nil
	}
	return false, "", nil
}

func hasPinentryOutsideBlock(content string) bool {
	body, _, _ := block.Remove(content)
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pinentry-program") {
			return true
		}
	}
	return false
}

func (in *Installer) writePinentry(ctx context.Context) (string, error) {
	result, err := in.Runner.Run(ctx, execx.Cmd{Name: "brew", Args: []string{"--prefix", "pinentry-mac"}})
	if err != nil {
		return "", err
	}
	prefix := strings.TrimSpace(string(result.Stdout))
	body := "pinentry-program " + filepath.Join(prefix, "bin", "pinentry-mac")
	if _, err := block.UpsertFile(in.Paths.GpgAgentConf(), body, in.now()); err != nil {
		return "", err
	}
	if _, err := in.Runner.Run(ctx, execx.Cmd{Name: "gpgconf", Args: []string{"--kill", "gpg-agent"}}); err != nil {
		return "", err
	}
	in.recordBlock(in.Paths.GpgAgentConf())
	return "pinentry-mac configured", nil
}

func (in *Installer) recordBlock(path string) {
	for _, existing := range in.Manifest.Blocks {
		if existing == path {
			return
		}
	}
	in.Manifest.Blocks = append(in.Manifest.Blocks, path)
}

func (in *Installer) opStep() Step {
	return Step{
		Name: "1password",
		Check: func(ctx context.Context) (bool, string, error) {
			_, err := in.Runner.Run(ctx, execx.Cmd{Name: "op", Args: []string{"whoami"}})
			if err != nil {
				in.Out.Report(Warning, "1password", "Enable 1Password > Settings > Developer > Integrate with 1Password CLI")
			}
			return true, "signed in", nil
		},
		Apply: func(context.Context) (string, error) { return "", nil },
	}
}
