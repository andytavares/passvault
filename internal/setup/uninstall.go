package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/atavares-klaviyo/pass-vault/internal/block"
	"github.com/atavares-klaviyo/pass-vault/internal/execx"
)

// UninstallOptions are the pvault uninstall flags.
type UninstallOptions struct {
	PurgeVault bool
	PurgeKey   bool
	KeepConfig bool
	DryRun     bool
}

// KeptItem is something uninstall leaves alone, with the reason.
type KeptItem struct {
	Item   string
	Reason string
}

// UninstallPlan is what uninstall will remove and what it will keep.
type UninstallPlan struct {
	Remove []string
	Keep   []KeptItem
}

// Plan lists the removals and keeps for opts without changing anything.
func (in *Installer) Plan(ctx context.Context, opts UninstallOptions) (UninstallPlan, error) {
	plan := UninstallPlan{}
	if err := in.planPackages(ctx, &plan); err != nil {
		return plan, err
	}
	in.planKey(&plan, opts)
	for _, file := range in.Manifest.Blocks {
		plan.Remove = append(plan.Remove, "managed block in "+file)
	}
	if !opts.KeepConfig {
		plan.Remove = append(plan.Remove, in.Paths.ConfigDir)
	} else {
		plan.Keep = append(plan.Keep, KeptItem{in.Paths.ConfigDir, "--keep-config"})
	}
	plan.Remove = append(plan.Remove, in.Paths.StateDir, in.Paths.Binary())
	if opts.PurgeVault {
		plan.Remove = append(plan.Remove, in.Paths.StoreDir)
	} else {
		plan.Keep = append(plan.Keep, KeptItem{in.Paths.StoreDir, "pass store; use --purge-vault to remove"})
	}
	return plan, nil
}

func (in *Installer) planPackages(ctx context.Context, plan *UninstallPlan) error {
	for _, pkg := range in.Manifest.Packages {
		users, err := in.outsideDependents(ctx, pkg.Name)
		if err != nil {
			return err
		}
		if users != "" {
			plan.Keep = append(plan.Keep, KeptItem{pkg.Name, "needed by " + users})
			continue
		}
		plan.Remove = append(plan.Remove, "brew package "+pkg.Name)
	}
	for _, pkg := range toolPackages {
		if in.Manifest.HasPackage(pkg.Name) || !in.brewInstalled(ctx, pkg) {
			continue
		}
		plan.Keep = append(plan.Keep, KeptItem{pkg.Name, "present before setup"})
	}
	return nil
}

func (in *Installer) planKey(plan *UninstallPlan, opts UninstallOptions) {
	key := in.Manifest.CreatedKey
	switch {
	case key == "" && opts.PurgeKey:
		plan.Keep = append(plan.Keep, KeptItem{"GPG key", "not created by pvault"})
	case key == "":
		plan.Keep = append(plan.Keep, KeptItem{"GPG key", "not created by pvault"})
	case opts.PurgeKey:
		plan.Remove = append(plan.Remove, "GPG key "+key)
	default:
		plan.Keep = append(plan.Keep, KeptItem{"GPG key " + key, "use --purge-key to remove"})
	}
}

// outsideDependents lists installed packages that need name and are not
// themselves in the manifest.
func (in *Installer) outsideDependents(ctx context.Context, name string) (string, error) {
	result, err := in.Runner.Run(ctx, execx.Cmd{Name: "brew", Args: []string{"uses", "--installed", name}})
	if err != nil {
		return "", err
	}
	var outside []string
	for _, user := range strings.Fields(string(result.Stdout)) {
		if !in.Manifest.HasPackage(user) {
			outside = append(outside, user)
		}
	}
	return strings.Join(outside, ", "), nil
}

func (in *Installer) brewInstalled(ctx context.Context, pkg Package) bool {
	_, err := in.Runner.Run(ctx, listCmd(pkg))
	return err == nil
}

// Uninstall reverses the manifest: blocks, packages, config, state, binary.
func (in *Installer) Uninstall(ctx context.Context, opts UninstallOptions) error {
	plan, err := in.Plan(ctx, opts)
	if err != nil {
		return err
	}
	if opts.PurgeVault {
		if err := in.confirmPurge(); err != nil {
			return err
		}
	}
	if opts.DryRun {
		return nil
	}
	if err := in.removeBlocks(); err != nil {
		return err
	}
	if err := in.removePackages(ctx, plan); err != nil {
		return err
	}
	return in.removeFiles(ctx, opts)
}

// confirmPurge makes the user type the store path before it is deleted.
func (in *Installer) confirmPurge() error {
	answer := in.Prompt.Ask("Type the store path to confirm vault deletion", "")
	if answer == in.Paths.StoreDir || answer == tildeForm(in.Paths.StoreDir, in.Paths.Home) {
		return nil
	}
	return fmt.Errorf("vault deletion aborted: %q does not match %s", answer, in.Paths.StoreDir)
}

func tildeForm(path, home string) string {
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return ""
}

func (in *Installer) removeBlocks() error {
	for _, file := range in.Manifest.Blocks {
		if _, err := block.RemoveFile(file); err != nil {
			return err
		}
	}
	return nil
}

func (in *Installer) removePackages(ctx context.Context, plan UninstallPlan) error {
	kept := map[string]bool{}
	for _, keep := range plan.Keep {
		kept[keep.Item] = true
	}
	for i := len(in.Manifest.Packages) - 1; i >= 0; i-- {
		pkg := in.Manifest.Packages[i]
		if kept[pkg.Name] {
			continue
		}
		args := []string{"uninstall"}
		if pkg.Cask {
			args = append(args, "--cask")
		}
		if _, err := in.Runner.Run(ctx, execx.Cmd{Name: "brew", Args: append(args, pkg.Name)}); err != nil {
			return err
		}
	}
	return nil
}

func (in *Installer) removeFiles(ctx context.Context, opts UninstallOptions) error {
	if opts.PurgeKey && in.Manifest.CreatedKey != "" {
		if err := in.deleteKey(ctx); err != nil {
			return err
		}
	}
	if opts.PurgeVault {
		if err := os.RemoveAll(in.Paths.StoreDir); err != nil {
			return err
		}
	}
	return in.removeDirs(opts)
}

func (in *Installer) removeDirs(opts UninstallOptions) error {
	if !opts.KeepConfig {
		if err := os.RemoveAll(in.Paths.ConfigDir); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(in.Paths.StateDir); err != nil {
		return err
	}
	if err := os.Remove(in.Paths.Binary()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (in *Installer) deleteKey(ctx context.Context) error {
	_, err := in.Runner.Run(ctx, execx.Cmd{Name: "gpg", Args: []string{
		"--batch", "--yes", "--delete-secret-and-public-key", in.Manifest.CreatedKey}})
	return err
}
