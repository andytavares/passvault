// Package testenv builds a throwaway HOME with a real GPG key, a real pass
// store, and fake op and brew programs, for tests in every package.
package testenv

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/execx"
	"github.com/atavares-klaviyo/pass-vault/internal/paths"
)

// Env is one isolated environment. Its fields are absolute paths.
type Env struct {
	Root        string
	Paths       paths.Paths
	FakeBin     string // on PATH before the real bins: op, brew
	OpFixtures  string // one file per op:// reference, see SetOp
	BrewState   string // one file per installed package name
	Fingerprint string // set by WithStore
}

// New sets HOME, XDG dirs, GNUPGHOME and PASSWORD_STORE_DIR to a temp root
// and puts fake op and brew first on PATH. Cleanup kills the gpg-agent.
func New(t *testing.T) *Env {
	t.Helper()
	// gpg-agent socket paths must stay under ~104 bytes on macOS, so avoid
	// t.TempDir, whose path includes the test name.
	root, err := os.MkdirTemp("", "pv")
	if err != nil {
		t.Fatal(err)
	}
	root, _ = filepath.EvalSymlinks(root)
	env := &Env{
		Root:       root,
		FakeBin:    filepath.Join(root, "bin"),
		OpFixtures: filepath.Join(root, "op"),
		BrewState:  filepath.Join(root, "brew"),
	}
	env.makeDirs(t)
	env.setEnv(t)
	env.Paths = paths.FromEnv()
	t.Cleanup(func() {
		_ = exec.Command("gpgconf", "--kill", "all").Run()
		_ = os.RemoveAll(root)
	})
	return env
}

func (e *Env) makeDirs(t *testing.T) {
	for _, dir := range []string{e.FakeBin, e.OpFixtures, e.BrewState, filepath.Join(e.Root, "brew-uses")} {
		mustMkdir(t, dir, 0o755)
	}
	mustMkdir(t, filepath.Join(e.Root, "g"), 0o700)
	writeFile(t, filepath.Join(e.Root, "gitconfig"), "[user]\n\tname = pvault test\n\temail = t@example.invalid\n")
	writeExecutable(t, filepath.Join(e.FakeBin, "op"), fakeOp)
	writeExecutable(t, filepath.Join(e.FakeBin, "brew"), fakeBrew)
}

var (
	goDirsOnce sync.Once
	goDirs     [2]string // GOMODCACHE, GOCACHE of the real user
)

// pinGoDirs keeps `go build` inside tests using the real module and build
// caches, because HOME points at an empty temp directory.
func pinGoDirs(t *testing.T) {
	goDirsOnce.Do(func() {
		out, err := exec.Command("go", "env", "GOMODCACHE", "GOCACHE").Output()
		if err != nil {
			return
		}
		copy(goDirs[:], strings.Split(strings.TrimSpace(string(out)), "\n"))
	})
	t.Setenv("GOMODCACHE", goDirs[0])
	t.Setenv("GOCACHE", goDirs[1])
	t.Setenv("GOFLAGS", "-mod=readonly")
}

func (e *Env) setEnv(t *testing.T) {
	root := e.Root
	pinGoDirs(t)
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, ".local", "state"))
	t.Setenv("GNUPGHOME", filepath.Join(root, "g"))
	t.Setenv("PASSWORD_STORE_DIR", filepath.Join(root, "store"))
	t.Setenv("PATH", e.FakeBin+":"+os.Getenv("PATH"))
	t.Setenv("PVAULT_FAKE_ROOT", root)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "gitconfig"))
}

// WithStore creates a GPG key with no passphrase and runs pass init with it.
func (e *Env) WithStore(t *testing.T) *Env {
	t.Helper()
	e.GenerateKey(t)
	e.mustRun(t, "pass", "init", e.Fingerprint)
	return e
}

// GenerateKey creates an ed25519/cv25519 key with no passphrase.
func (e *Env) GenerateKey(t *testing.T) {
	t.Helper()
	e.mustRun(t, "gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase", "",
		"--quick-generate-key", "pvault test <t@example.invalid>", "future-default", "default", "never")
	out := e.mustRun(t, "gpg", "--list-secret-keys", "--with-colons")
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "fpr:") {
			e.Fingerprint = strings.Split(line, ":")[9]
			return
		}
	}
	t.Fatal("no fingerprint after key generation")
}

// WithGit runs pass git init on the store.
func (e *Env) WithGit(t *testing.T) *Env {
	t.Helper()
	e.mustRun(t, "pass", "git", "init")
	return e
}

// Insert writes a pass entry with the exact bytes given.
func (e *Env) Insert(t *testing.T, name, value string) {
	t.Helper()
	cmd := exec.Command("pass", "insert", "-m", "-f", name)
	cmd.Stdin = strings.NewReader(value)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pass insert %s: %v\n%s", name, err, out)
	}
}

// Show returns the exact bytes of a pass entry.
func (e *Env) Show(t *testing.T, name string) string {
	t.Helper()
	out, err := exec.Command("pass", "show", name).Output()
	if err != nil {
		t.Fatalf("pass show %s: %v", name, err)
	}
	return string(out)
}

// SetOp makes the fake `op read -n ref` print value.
func (e *Env) SetOp(t *testing.T, ref, value string) {
	t.Helper()
	writeFile(t, filepath.Join(e.OpFixtures, FixtureKey(ref)), value)
}

// SetOpSignedIn controls whether the fake `op whoami` succeeds.
func (e *Env) SetOpSignedIn(t *testing.T, signedIn bool) {
	t.Helper()
	marker := filepath.Join(e.Root, "op-signed-in")
	if signedIn {
		writeFile(t, marker, "")
		return
	}
	_ = os.Remove(marker)
}

// SetOpItems sets the JSON the fake `op item list --format json` prints.
func (e *Env) SetOpItems(t *testing.T, json string) {
	t.Helper()
	writeFile(t, filepath.Join(e.Root, "op-items.json"), json)
}

// SetBrewInstalled marks a fake brew package as installed.
func (e *Env) SetBrewInstalled(t *testing.T, name, version string) {
	t.Helper()
	writeFile(t, filepath.Join(e.BrewState, name), version)
}

// SetBrewUses sets what `brew uses --installed name` prints.
func (e *Env) SetBrewUses(t *testing.T, name string, users ...string) {
	t.Helper()
	writeFile(t, filepath.Join(e.Root, "brew-uses", name), strings.Join(users, "\n"))
}

// BrewInstalled reports whether the fake brew has the package.
func (e *Env) BrewInstalled(name string) bool {
	_, err := os.Stat(filepath.Join(e.BrewState, name))
	return err == nil
}

// Log returns the argv lines the fake program (op or brew) received.
func (e *Env) Log(t *testing.T, program string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(e.Root, program+".log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// Runner returns a recording runner over the real OS runner.
func (e *Env) Runner() *execx.Recorder {
	return &execx.Recorder{Inner: execx.OS{}}
}

// FixtureKey is the file name the fake op uses for a reference.
func FixtureKey(ref string) string {
	sum := sha1.Sum([]byte(ref))
	return hex.EncodeToString(sum[:])[:16]
}

func (e *Env) mustRun(t *testing.T, name string, args ...string) string {
	t.Helper()
	result, err := execx.OS{}.Run(context.Background(), execx.Cmd{Name: name, Args: args})
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return string(result.Stdout)
}

func mustMkdir(t *testing.T, dir string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(dir, mode); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

const fakeOp = `#!/bin/sh
root="$PVAULT_FAKE_ROOT"
echo "$*" >> "$root/op.log"
case "$1" in
  whoami)
    [ -f "$root/op-signed-in" ] && { echo "URL: fake"; exit 0; }
    echo "[ERROR] account is not signed in" >&2; exit 1 ;;
  read)
    shift
    [ "$1" = "-n" ] && shift
    key=$(printf %s "$1" | shasum | cut -c1-16)
    [ -f "$root/op/$key" ] || { echo "[ERROR] could not read secret '$1': not found" >&2; exit 1; }
    cat "$root/op/$key" ;;
  item)
    [ "$2" = "list" ] && { cat "$root/op-items.json" 2>/dev/null || echo '[]'; exit 0; }
    [ "$2" = "get" ] && { cat "$root/op-item-$3.json" 2>/dev/null || { echo "[ERROR] item not found" >&2; exit 1; }; exit 0; }
    exit 1 ;;
  --version) echo "2.41.0" ;;
  *) echo "fake op: unsupported $*" >&2; exit 2 ;;
esac
`

const fakeBrew = `#!/bin/sh
root="$PVAULT_FAKE_ROOT"
echo "$*" >> "$root/brew.log"
cmd="$1"; shift
[ "$1" = "--cask" ] && shift
[ "$1" = "--versions" ] && shift
case "$cmd" in
  list)
    [ -f "$root/brew/$1" ] || exit 1
    echo "$1 $(cat "$root/brew/$1")" ;;
  install) echo "1.0" > "$root/brew/$1" ;;
  uninstall) rm -f "$root/brew/$1" ;;
  upgrade) [ -f "$root/brew/$1" ] && echo "1.1" > "$root/brew/$1" ;;
  uses)
    [ "$1" = "--installed" ] && shift
    cat "$root/brew-uses/$1" 2>/dev/null; exit 0 ;;
  # pass's macOS platform.sh runs "brew --prefix gnu-getopt"; answer with
  # the real path or pass falls back to BSD getopt and loops forever.
  --prefix) if [ -n "$1" ]; then echo "/opt/homebrew/opt/$1"; else echo "/opt/homebrew"; fi ;;
  *) echo "fake brew: unsupported $cmd" >&2; exit 2 ;;
esac
`
