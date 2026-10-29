package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/atavares-klaviyo/pass-vault/internal/execx"
)

// UpgradeOptions are the pvault upgrade flags.
type UpgradeOptions struct {
	NoTools bool
}

const (
	defaultRepoURL = "https://github.com/andytavares/passvault.git"
	versionLdflag  = "-X github.com/atavares-klaviyo/pass-vault/internal/cli.Version="
)

// Upgrade pulls the source clone, rebuilds and atomically swaps the binary,
// then upgrades the brew toolset unless NoTools.
func (in *Installer) Upgrade(ctx context.Context, opts UpgradeOptions) error {
	if in.Manifest.SourceDir == "" {
		if err := in.cloneSource(ctx); err != nil {
			return err
		}
	} else if err := in.pullSource(ctx); err != nil {
		return err
	}
	if err := in.rebuildBinary(ctx); err != nil {
		return err
	}
	if !opts.NoTools {
		in.upgradeTools(ctx)
	}
	return nil
}

// cloneSource clones the repo into the state dir and saves it as the source
// clone, so later upgrades pull it.
func (in *Installer) cloneSource(ctx context.Context) error {
	url := in.RepoURL
	if url == "" {
		url = defaultRepoURL
	}
	dir := filepath.Join(in.Paths.StateDir, "source")
	if _, err := in.Runner.Run(ctx, execx.Cmd{Name: "git", Args: []string{"clone", url, dir}}); err != nil {
		return err
	}
	in.Manifest.SourceDir = dir
	return in.Manifest.Save(in.Paths.Manifest())
}

func (in *Installer) pullSource(ctx context.Context) error {
	_, err := in.Runner.Run(ctx, execx.Cmd{
		Name: "git", Args: []string{"-C", in.Manifest.SourceDir, "pull", "--ff-only"}})
	return err
}

func (in *Installer) rebuildBinary(ctx context.Context) error {
	version, err := in.sourceVersion(ctx)
	if err != nil {
		return err
	}
	target := in.Paths.Binary() + ".new"
	_, err = in.Runner.Run(ctx, execx.Cmd{
		Name: "go", Dir: in.Manifest.SourceDir,
		Args: []string{"build", "-ldflags", versionLdflag + version, "-o", target, "./cmd/pvault"}})
	if err != nil {
		return err
	}
	return os.Rename(target, in.Paths.Binary())
}

func (in *Installer) sourceVersion(ctx context.Context) (string, error) {
	result, err := in.Runner.Run(ctx, execx.Cmd{
		Name: "git", Dir: in.Manifest.SourceDir,
		Args: []string{"describe", "--tags", "--always", "--dirty"}})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(result.Stdout)), nil
}

// upgradeTools runs brew upgrade for each installed tool; an "already
// installed" failure is a warning, not an error.
func (in *Installer) upgradeTools(ctx context.Context) {
	for _, pkg := range toolPackages {
		if !in.brewInstalled(ctx, pkg) {
			continue
		}
		args := []string{"upgrade"}
		if pkg.Cask {
			args = append(args, "--cask")
		}
		if _, err := in.Runner.Run(ctx, execx.Cmd{Name: "brew", Args: append(args, pkg.Name)}); err != nil {
			in.Out.Report(Warning, "upgrade "+pkg.Name, fmt.Sprintf("brew upgrade failed: %v", err))
		}
	}
}
