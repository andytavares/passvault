package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/atavares-klaviyo/pass-vault/internal/execx"
)

// GpgListSecretKeys prints the secret keyring in colon format.
func GpgListSecretKeys() execx.Cmd {
	return execx.Cmd{Name: "gpg", Args: []string{"--list-secret-keys", "--with-colons"}}
}

// secretFingerprint returns the fingerprint of the first secret key.
func (in *Installer) secretFingerprint(ctx context.Context) (string, error) {
	result, err := in.Runner.Run(ctx, GpgListSecretKeys())
	if err != nil {
		var exitErr *execx.ExitError
		if errors.As(err, &exitErr) {
			return "", nil
		}
		return "", err
	}
	for _, line := range strings.Split(string(result.Stdout), "\n") {
		if strings.HasPrefix(line, "fpr:") {
			return strings.Split(line, ":")[9], nil
		}
	}
	return "", nil
}

func (in *Installer) keyStep() Step {
	return Step{
		Name:  "gpg key",
		Check: func(ctx context.Context) (bool, string, error) { return in.keyDone(ctx) },
		Apply: func(ctx context.Context) (string, error) { return in.createKey(ctx) },
	}
}

func (in *Installer) keyDone(ctx context.Context) (bool, string, error) {
	fingerprint, err := in.secretFingerprint(ctx)
	if err != nil || fingerprint == "" {
		return false, "", err
	}
	return true, "key " + fingerprint, nil
}

func (in *Installer) createKey(ctx context.Context) (string, error) {
	if !in.Prompt.Confirm("No GPG secret key found. Create one now?") {
		return "", errors.New("pvault needs a GPG key")
	}
	name := in.Prompt.Ask("Your name for the key", in.gitConfig(ctx, "user.name"))
	email := in.Prompt.Ask("Your email for the key", in.gitConfig(ctx, "user.email"))
	if err := in.generateKey(ctx, name+" <"+email+">"); err != nil {
		return "", err
	}
	fingerprint, err := in.secretFingerprint(ctx)
	if err != nil || fingerprint == "" {
		return "", errors.New("no fingerprint after key generation")
	}
	in.Manifest.CreatedKey = fingerprint
	return "created key " + fingerprint, nil
}

// generateKey runs the interactive gpg call; tests inject a batch version.
func (in *Installer) generateKey(ctx context.Context, userID string) error {
	if in.GenerateKey != nil {
		return in.GenerateKey(ctx, userID)
	}
	_, err := in.Runner.Run(ctx, execx.Cmd{
		Name: "gpg", Terminal: true,
		Args: []string{"--quick-generate-key", userID, "future-default", "default", "2y"},
	})
	return err
}

func (in *Installer) gitConfig(ctx context.Context, key string) string {
	result, err := in.Runner.Run(ctx, execx.Cmd{Name: "git", Args: []string{"config", key}})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(result.Stdout))
}

func (in *Installer) storeStep() Step {
	return Step{
		Name:  "pass store",
		Check: func(ctx context.Context) (bool, string, error) { return in.storeDone(ctx) },
		Apply: func(ctx context.Context) (string, error) { return in.initStore(ctx) },
	}
}

func (in *Installer) storeDone(ctx context.Context) (bool, string, error) {
	data, err := os.ReadFile(filepath.Join(in.Paths.StoreDir, ".gpg-id"))
	if os.IsNotExist(err) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	for _, id := range strings.Fields(string(data)) {
		if _, err := in.Runner.Run(ctx, execx.Cmd{Name: "gpg", Args: []string{"--list-secret-keys", id}}); err != nil {
			in.Out.Report(Warning, "pass store", "no secret key for .gpg-id "+id)
		}
	}
	return true, "store present", nil
}

func (in *Installer) initStore(ctx context.Context) (string, error) {
	fingerprint, err := in.secretFingerprint(ctx)
	if err != nil || fingerprint == "" {
		return "", errors.New("pvault needs a GPG key")
	}
	if _, err := in.Runner.Run(ctx, execx.Cmd{Name: "pass", Args: []string{"init", fingerprint}}); err != nil {
		return "", err
	}
	in.Manifest.InitializedStore = true
	return "pass init " + fingerprint, nil
}

func (in *Installer) gitStep() Step {
	return Step{
		Name: "store git",
		Check: func(context.Context) (bool, string, error) {
			if _, err := os.Stat(filepath.Join(in.Paths.StoreDir, ".git")); os.IsNotExist(err) {
				return false, "", nil
			}
			return true, "git repo present", nil
		},
		Apply: func(ctx context.Context) (string, error) {
			if _, err := in.Runner.Run(ctx, execx.Cmd{Name: "pass", Args: []string{"git", "init"}}); err != nil {
				return "", err
			}
			in.Manifest.InitializedGit = true
			return "pass git init", nil
		},
	}
}
