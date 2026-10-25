package setup_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/execx"
	"github.com/atavares-klaviyo/pass-vault/internal/setup"
	"github.com/atavares-klaviyo/pass-vault/internal/testenv"
)

func runSetup(t *testing.T, env *testenv.Env) *setup.Manifest {
	t.Helper()
	inst, _ := newInstaller(env, &setup.Manifest{})
	if err := inst.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved, err := setup.LoadManifest(env.Paths.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func uninstaller(env *testenv.Env, manifest *setup.Manifest, answers map[string]string) *setup.Installer {
	inst, _ := newInstaller(env, manifest)
	inst.Prompt = fakePrompt{confirm: true, answers: answers}
	return inst
}

func TestUninstallRemovesOnlyManifestPackagesReverseOrder(t *testing.T) {
	env := testenv.New(t)
	env.SetBrewInstalled(t, "htop", "3.0")
	writeFile(t, env.Paths.Zshrc(), "export A=1\n")
	manifest := runSetup(t, env)
	before := len(brewCalls(t, env, "uninstall"))
	inst := uninstaller(env, manifest, nil)
	if err := inst.Uninstall(context.Background(), setup.UninstallOptions{}); err != nil {
		t.Fatal(err)
	}
	got := brewCalls(t, env, "uninstall")[before:]
	want := "uninstall --cask 1password-cli,uninstall pinentry-mac,uninstall gnupg,uninstall pass"
	if strings.Join(got, ",") != want {
		t.Fatalf("uninstalls %v", got)
	}
	if !env.BrewInstalled("htop") {
		t.Fatal("htop removed")
	}
	assertUninstallRemoved(t, env)
}

func assertUninstallRemoved(t *testing.T, env *testenv.Env) {
	t.Helper()
	data, _ := os.ReadFile(env.Paths.Zshrc())
	if string(data) != "export A=1\n" {
		t.Fatalf("zshrc %q", data)
	}
	for _, path := range []string{env.Paths.ConfigDir, env.Paths.StateDir, env.Paths.Binary()} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s still exists", path)
		}
	}
	if _, err := os.Stat(env.Paths.StoreDir); err != nil {
		t.Error("store removed without PurgeVault")
	}
}

func TestUninstallKeepsPackageWithOutsideDependent(t *testing.T) {
	env := testenv.New(t)
	env.SetBrewInstalled(t, "other-tool", "2.0")
	env.SetBrewUses(t, "gnupg", "other-tool")
	manifest := runSetup(t, env)
	plan, err := uninstaller(env, manifest, nil).Plan(context.Background(), setup.UninstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKeep(plan, "gnupg", "needed by other-tool") {
		t.Fatalf("plan %+v", plan)
	}
	if !hasKeep(plan, "gnupg", "needed by other-tool") || len(plan.Keep) < 2 {
		t.Fatalf("plan %+v", plan)
	}
	before := len(brewCalls(t, env, "uninstall"))
	if err := uninstaller(env, manifest, nil).Uninstall(context.Background(), setup.UninstallOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, line := range brewCalls(t, env, "uninstall")[before:] {
		if line == "uninstall gnupg" {
			t.Fatal("gnupg uninstalled")
		}
	}
	if !env.BrewInstalled("gnupg") {
		t.Fatal("gnupg removed")
	}
}

func hasKeep(plan setup.UninstallPlan, item, reason string) bool {
	for _, keep := range plan.Keep {
		if strings.Contains(keep.Item, item) && strings.Contains(keep.Reason, reason) {
			return true
		}
	}
	return false
}

func TestPurgeVaultNeedsExactConfirmation(t *testing.T) {
	env := testenv.New(t)
	manifest := runSetup(t, env)
	answers := map[string]string{"Type the store path to confirm vault deletion": "/wrong"}
	inst := uninstaller(env, manifest, answers)
	err := inst.Uninstall(context.Background(), setup.UninstallOptions{PurgeVault: true})
	if err == nil {
		t.Fatal("no error")
	}
	if _, err := os.Stat(env.Paths.StoreDir); err != nil {
		t.Fatal("store deleted")
	}
	tildePath := "~" + strings.TrimPrefix(env.Paths.StoreDir, env.Paths.Home)
	tilde := map[string]string{"Type the store path to confirm vault deletion": tildePath}
	inst = uninstaller(env, manifest, tilde)
	if err := inst.Uninstall(context.Background(), setup.UninstallOptions{PurgeVault: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.Paths.StoreDir); !os.IsNotExist(err) {
		t.Fatal("store kept after confirmation")
	}
}

func TestPurgeKeyDeletesOnlyCreatedKey(t *testing.T) {
	env := testenv.New(t)
	manifest := runSetup(t, env)
	created := manifest.CreatedKey
	if created == "" {
		t.Fatal("no created key")
	}
	other := generateKey(t, "other user <o@example.invalid>")
	inst := uninstaller(env, manifest, nil)
	if err := inst.Uninstall(context.Background(), setup.UninstallOptions{PurgeKey: true}); err != nil {
		t.Fatal(err)
	}
	out := secretKeyOutput(t, env)
	if strings.Contains(out, created) {
		t.Fatal("created key survived")
	}
	if !strings.Contains(out, other) {
		t.Fatal("other key deleted")
	}
}

func TestPurgeKeyKeepsForeignKey(t *testing.T) {
	env := testenv.New(t)
	preexisting := generateKey(t, "old user <old@example.invalid>")
	manifest := runSetup(t, env)
	manifest.CreatedKey = ""
	inst := uninstaller(env, manifest, nil)
	plan, err := inst.Plan(context.Background(), setup.UninstallOptions{PurgeKey: true})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKeep(plan, "GPG key", "not created by pvault") {
		t.Fatalf("plan %+v", plan)
	}
	if err := inst.Uninstall(context.Background(), setup.UninstallOptions{PurgeKey: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(secretKeyOutput(t, env), preexisting) {
		t.Fatal("pre-existing key deleted")
	}
}

func generateKey(t *testing.T, userID string) string {
	t.Helper()
	if err := batchKey(context.Background(), userID); err != nil {
		t.Fatal(err)
	}
	result, err := execx.OS{}.Run(context.Background(), setup.GpgListSecretKeys())
	if err != nil {
		t.Fatal(err)
	}
	return lastFingerprint(t, string(result.Stdout))
}

func lastFingerprint(t *testing.T, colonOutput string) string {
	t.Helper()
	fingerprint := ""
	for _, line := range strings.Split(colonOutput, "\n") {
		if strings.HasPrefix(line, "fpr:") {
			fingerprint = strings.Split(line, ":")[9]
		}
	}
	if fingerprint == "" {
		t.Fatal("no fingerprint")
	}
	return fingerprint
}

func secretKeyOutput(t *testing.T, env *testenv.Env) string {
	t.Helper()
	result, err := env.Runner().Run(context.Background(), setup.GpgListSecretKeys())
	if err != nil {
		t.Fatal(err)
	}
	return string(result.Stdout)
}

func TestUninstallKeepConfig(t *testing.T) {
	env := testenv.New(t)
	manifest := runSetup(t, env)
	inst := uninstaller(env, manifest, nil)
	if err := inst.Uninstall(context.Background(), setup.UninstallOptions{KeepConfig: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.Paths.ConfigDir); err != nil {
		t.Error("config removed despite KeepConfig")
	}
	if _, err := os.Stat(filepath.Join(env.Paths.StateDir)); !os.IsNotExist(err) {
		t.Error("state kept")
	}
}
