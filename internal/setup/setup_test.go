package setup_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atavares-klaviyo/pass-vault/internal/block"
	"github.com/atavares-klaviyo/pass-vault/internal/execx"
	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/setup"
	"github.com/atavares-klaviyo/pass-vault/internal/testenv"
)

var fixedNow = time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)

type report struct {
	status setup.Status
	name   string
	detail string
}

type reporter struct{ entries []report }

func (r *reporter) Report(status setup.Status, name, detail string) {
	r.entries = append(r.entries, report{status: status, name: name, detail: detail})
}

func (r *reporter) has(status setup.Status, substr string) bool {
	for _, e := range r.entries {
		if e.status == status && strings.Contains(e.detail, substr) {
			return true
		}
	}
	return false
}

type fakePrompt struct {
	confirm bool
	answers map[string]string
}

func (p fakePrompt) Confirm(string) bool { return p.confirm }

func (p fakePrompt) Ask(question, fallback string) string {
	if answer, ok := p.answers[question]; ok {
		return answer
	}
	return fallback
}

func batchKey(ctx context.Context, userID string) error {
	_, err := execx.OS{}.Run(ctx, execx.Cmd{Name: "gpg", Args: []string{
		"--batch", "--pinentry-mode", "loopback", "--passphrase", "",
		"--quick-generate-key", userID, "future-default", "default", "never"}})
	return err
}

func newInstaller(env *testenv.Env, manifest *setup.Manifest) (*setup.Installer, *reporter) {
	rep := &reporter{}
	inst := &setup.Installer{
		Runner:      env.Runner(),
		Paths:       env.Paths,
		Manifest:    manifest,
		Prompt:      fakePrompt{confirm: true},
		Out:         rep,
		SourceDir:   "/src",
		GOOS:        "darwin",
		GenerateKey: batchKey,
		Now:         func() time.Time { return fixedNow },
	}
	return inst, rep
}

func brewCalls(t *testing.T, env *testenv.Env, verb string) []string {
	t.Helper()
	var out []string
	for _, line := range env.Log(t, "brew") {
		if line == verb || strings.HasPrefix(line, verb+" ") {
			out = append(out, line)
		}
	}
	return out
}

func packageNames(manifest *setup.Manifest) []string {
	var out []string
	for _, pkg := range manifest.Packages {
		out = append(out, pkg.Name)
	}
	return out
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSetupInstallsEverythingOnFreshHome(t *testing.T) {
	env := testenv.New(t)
	manifest := &setup.Manifest{}
	inst, _ := newInstaller(env, manifest)
	if err := inst.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "install pass,install gnupg,install pinentry-mac,install --cask 1password-cli"
	if got := strings.Join(brewCalls(t, env, "install"), ","); got != want {
		t.Fatalf("installs %s", got)
	}
	if got := strings.Join(packageNames(manifest), ","); got != "pass,gnupg,pinentry-mac,1password-cli" {
		t.Fatalf("manifest packages %s", got)
	}
	assertSetupApplied(t, env)
}

func assertSetupApplied(t *testing.T, env *testenv.Env) {
	t.Helper()
	for _, name := range []string{"pass", "gnupg", "pinentry-mac", "1password-cli"} {
		if !env.BrewInstalled(name) {
			t.Errorf("%s not installed", name)
		}
	}
	for _, path := range []string{
		filepath.Join(env.Paths.StoreDir, ".gpg-id"),
		filepath.Join(env.Paths.StoreDir, ".git"),
		env.Paths.Mapping(), env.Paths.Manifest(),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing %s", path)
		}
	}
	saved, err := setup.LoadManifest(env.Paths.Manifest())
	if err != nil || saved.SchemaVersion != setup.CurrentSchema || saved.CreatedKey == "" || saved.SourceDir != "/src" {
		t.Fatalf("saved %+v err %v", saved, err)
	}
}

func TestSetupSecondRunChangesNothing(t *testing.T) {
	env := testenv.New(t)
	inst, _ := newInstaller(env, &setup.Manifest{})
	if err := inst.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved, _ := setup.LoadManifest(env.Paths.Manifest())
	again, rep := newInstaller(env, saved)
	if err := again.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := brewCalls(t, env, "install"); len(got) != 4 {
		t.Fatalf("second run installed %v", got)
	}
	if !rep.has(setup.Present, "managed") || rep.has(setup.Failed, "") {
		t.Fatalf("entries %+v", rep.entries)
	}
}

func TestPreInstalledPassStaysOutOfManifest(t *testing.T) {
	env := testenv.New(t)
	env.SetBrewInstalled(t, "pass", "1.8.0")
	manifest := &setup.Manifest{}
	inst, rep := newInstaller(env, manifest)
	if err := inst.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manifest.HasPackage("pass") {
		t.Fatal("pass in manifest")
	}
	if !rep.has(setup.Present, "present, not managed") {
		t.Fatalf("entries %+v", rep.entries)
	}
}

func TestUserPinentryLineIsLeftAlone(t *testing.T) {
	env := testenv.New(t)
	original := "pinentry-program /usr/local/bin/pinentry\n"
	writeFile(t, env.Paths.GpgAgentConf(), original)
	inst, rep := newInstaller(env, &setup.Manifest{})
	if err := inst.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(env.Paths.GpgAgentConf())
	if string(data) != original {
		t.Fatalf("conf changed to %q", data)
	}
	if !rep.has(setup.Warning, "pinentry-program") {
		t.Fatalf("entries %+v", rep.entries)
	}
}

func TestZshrcBlockBackupWarningAndIdempotence(t *testing.T) {
	env := testenv.New(t)
	original := "export A=1\nexport X=$(pass x)\n"
	writeFile(t, env.Paths.Zshrc(), original)
	inst, rep := newInstaller(env, &setup.Manifest{})
	if err := inst.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(env.Paths.Zshrc())
	if !block.Has(string(data)) {
		t.Fatal("no block")
	}
	backup, err := os.ReadFile(env.Paths.Zshrc() + ".pvault-20261025T130000")
	if err != nil || string(backup) != original {
		t.Fatalf("backup %q err %v", backup, err)
	}
	if !rep.has(setup.Warning, ":2 runs pass") {
		t.Fatalf("entries %+v", rep.entries)
	}
	saved, _ := setup.LoadManifest(env.Paths.Manifest())
	again, _ := newInstaller(env, saved)
	if err := again.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	rerun, _ := os.ReadFile(env.Paths.Zshrc())
	if string(rerun) != string(data) {
		t.Fatal("zshrc changed on rerun")
	}
}

func TestMappingExampleWrittenOnce(t *testing.T) {
	env := testenv.New(t)
	inst, _ := newInstaller(env, &setup.Manifest{})
	if err := inst.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(env.Paths.Mapping())
	if string(data) != mapping.Example {
		t.Fatal("mapping != example")
	}
	writeFile(t, env.Paths.Mapping(), "custom")
	saved, _ := setup.LoadManifest(env.Paths.Manifest())
	again, _ := newInstaller(env, saved)
	if err := again.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(env.Paths.Mapping())
	if string(data) != "custom" {
		t.Fatal("mapping overwritten")
	}
}

func TestDeclinedKeyCreationFails(t *testing.T) {
	env := testenv.New(t)
	inst, rep := newInstaller(env, &setup.Manifest{})
	inst.Prompt = fakePrompt{confirm: false}
	err := inst.Setup(context.Background())
	if err == nil || !strings.Contains(err.Error(), "pvault needs a GPG key") {
		t.Fatalf("err %v", err)
	}
	assertKeyDeclined(t, env, rep)
}

func assertKeyDeclined(t *testing.T, env *testenv.Env, rep *reporter) {
	t.Helper()
	if !rep.has(setup.Failed, "pvault needs a GPG key") {
		t.Fatalf("entries %+v", rep.entries)
	}
	if _, err := os.Stat(filepath.Join(env.Paths.StoreDir, ".gpg-id")); !os.IsNotExist(err) {
		t.Fatal("store initialised")
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	env := testenv.New(t)
	before := hashTree(t, env)
	inst, _ := newInstaller(env, &setup.Manifest{})
	inst.DryRun = true
	if err := inst.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if after := hashTree(t, env); after != before {
		t.Fatal("dry run changed files")
	}
	if _, err := os.Stat(env.Paths.Manifest()); !os.IsNotExist(err) {
		t.Fatal("manifest written")
	}
}

// hashTree skips the fake brew/op logs and gpg's own keybox housekeeping:
// those are test harness and gpg writes, not pvault writes.
func hashTree(t *testing.T, env *testenv.Env) string {
	t.Helper()
	digest := sha256.New()
	err := filepath.Walk(env.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(env.Root, path)
		if rel == "brew.log" || rel == "op.log" || strings.HasPrefix(rel, "g/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest.Write([]byte(rel))
		digest.Write(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func TestNonDarwinFails(t *testing.T) {
	env := testenv.New(t)
	inst, _ := newInstaller(env, &setup.Manifest{})
	inst.GOOS = "linux"
	if err := inst.Setup(context.Background()); err == nil {
		t.Fatal("no error")
	}
}

// pvault upgrade runs setup without --source; the saved clone path must stay.
func TestSetupWithoutSourceKeepsSavedSourceDir(t *testing.T) {
	env := testenv.New(t)
	inst, _ := newInstaller(env, &setup.Manifest{SourceDir: "/clone"})
	inst.SourceDir = ""
	if err := inst.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved, err := setup.LoadManifest(env.Paths.Manifest())
	if err != nil || saved.SourceDir != "/clone" {
		t.Fatalf("source_dir = %q, err %v; want /clone kept", saved.SourceDir, err)
	}
}
