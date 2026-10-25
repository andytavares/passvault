package setup_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/setup"
	"github.com/atavares-klaviyo/pass-vault/internal/testenv"
)

// makeSourceClone builds a git clone whose worktree looks enough like the
// repo for `go build ./cmd/pvault` to succeed: it links the real sources in.
func makeSourceClone(t *testing.T, env *testenv.Env) string {
	t.Helper()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(env.Root, "clone")
	remote := filepath.Join(env.Root, "remote")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cmd", "internal", "go.mod", "go.sum"} {
		if err := os.Symlink(filepath.Join(repo, name), filepath.Join(source, name)); err != nil {
			t.Fatal(err)
		}
	}
	run(t, source, []string{"git", "init"})
	run(t, source, []string{"git", "commit", "--allow-empty", "-m", "init"})
	run(t, env.Root, []string{"git", "clone", "--bare", source, remote})
	run(t, source, []string{"git", "remote", "add", "origin", remote})
	run(t, source, []string{"git", "fetch", "origin"})
	branch := strings.TrimSpace(string(gitOut(t, source, "rev-parse", "--abbrev-ref", "HEAD")))
	run(t, source, []string{"git", "branch", "--set-upstream-to", "origin/" + branch, branch})
	return source
}

func gitOut(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func run(t *testing.T, dir string, argv []string) {
	t.Helper()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", argv, err, out)
	}
}

func TestUpgradeWithoutToolsMakesNoBrewCalls(t *testing.T) {
	env := testenv.New(t)
	env.SetBrewInstalled(t, "pass", "1.0")
	source := makeSourceClone(t, env)
	manifest := &setup.Manifest{SourceDir: source}
	inst, _ := newInstaller(env, manifest)
	if err := inst.Upgrade(context.Background(), setup.UpgradeOptions{NoTools: true}); err != nil {
		t.Fatal(err)
	}
	for _, line := range env.Log(t, "brew") {
		if strings.HasPrefix(line, "upgrade") {
			t.Fatalf("brew upgrade ran: %s", line)
		}
	}
	if _, err := os.Stat(env.Paths.Binary()); err != nil {
		t.Fatal("binary not swapped in")
	}
}

func TestUpgradeRunsBrewUpgradeForInstalledTools(t *testing.T) {
	env := testenv.New(t)
	env.SetBrewInstalled(t, "pass", "1.0")
	env.SetBrewInstalled(t, "1password-cli", "2.0")
	source := makeSourceClone(t, env)
	inst, _ := newInstaller(env, &setup.Manifest{SourceDir: source})
	if err := inst.Upgrade(context.Background(), setup.UpgradeOptions{}); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(brewCalls(t, env, "upgrade"), ",")
	if got != "upgrade pass,upgrade --cask 1password-cli" {
		t.Fatalf("upgrades %s", got)
	}
}

func TestUpgradeWithoutSourceDirErrors(t *testing.T) {
	env := testenv.New(t)
	inst, _ := newInstaller(env, &setup.Manifest{})
	err := inst.Upgrade(context.Background(), setup.UpgradeOptions{NoTools: true})
	if err == nil || !strings.Contains(err.Error(), "unknown source clone") {
		t.Fatalf("err %v", err)
	}
}
