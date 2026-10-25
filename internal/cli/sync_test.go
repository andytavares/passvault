package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/testenv"
)

// cliFixture is one test environment with the app wired to buffers.
type cliFixture struct {
	env    *testenv.Env
	app    *app
	out    *bytes.Buffer
	errOut *bytes.Buffer
}

func newCliFixture(t *testing.T, stdin string) *cliFixture {
	t.Helper()
	env := testenv.New(t).WithStore(t).WithGit(t)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	app := &app{
		paths:  env.Paths,
		runner: env.Runner(),
		in:     strings.NewReader(stdin),
		out:    out, errOut: errOut,
		now: func() time.Time { return time.Date(2026, 10, 25, 13, 5, 12, 0, time.UTC) },
	}
	return &cliFixture{env: env, app: app, out: out, errOut: errOut}
}

// writeMapping writes secrets to the mapping file.
func (f *cliFixture) writeMapping(t *testing.T, secrets ...mapping.Secret) {
	t.Helper()
	content := "version = 1\n"
	for _, secret := range secrets {
		scope := string(secret.Scope)
		if scope == "" {
			scope = "global"
		}
		content += "\n[[secret]]\npass = \"" + secret.Pass + "\"\nref = \"" + secret.Ref + "\"\n"
		if secret.Env != "" {
			content += "env = \"" + secret.Env + "\"\n"
		}
		content += "scope = \"" + scope + "\"\n"
		for _, command := range secret.Commands {
			content += "commands = [\"" + command + "\"]\n"
		}
	}
	if err := os.MkdirAll(f.env.Paths.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.env.Paths.Mapping(), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// run executes cmd with args and returns the error (often an ExitCodeError).
func (f *cliFixture) run(cmd *cobra.Command, args ...string) error {
	cmd.SetArgs(args)
	return cmd.Execute()
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var codeErr ExitCodeError
	if errors.As(err, &codeErr) {
		return codeErr.Code
	}
	t.Fatalf("unexpected error: %v", err)
	return -1
}

func commitCount(t *testing.T, env *testenv.Env) int {
	t.Helper()
	out, err := exec.Command("git", "-C", env.Paths.StoreDir, "rev-list", "--count", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range strings.TrimSpace(string(out)) {
		n = n*10 + int(r-'0')
	}
	return n
}

var pagerduty = mapping.Secret{Pass: "pagerduty-api-key", Ref: "op://Private/PagerDuty/credential", Env: "PAGERDUTY_API_KEY"}

// syncOnce runs a sync of pagerduty and returns its error. step bumps the
// fake clock between runs so each sync gets its own tag.
func (f *cliFixture) syncOnce(t *testing.T, value string) error {
	t.Helper()
	f.env.SetOp(t, pagerduty.Ref, value)
	f.env.SetOpSignedIn(t, true)
	return f.run(f.app.syncCommand())
}

// tick moves the app's fake clock forward one second.
func (f *cliFixture) tick() {
	moment := f.app.now()
	f.app.now = func() time.Time { return moment.Add(time.Second) }
}

func TestSyncHappyPath(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)

	if err := f.syncOnce(t, "pd-secret-123"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !strings.Contains(f.out.String(), "+ pagerduty-api-key") {
		t.Fatalf("output missing add line:\n%s", f.out.String())
	}
	if got := f.env.Show(t, "pagerduty-api-key"); got != "pd-secret-123" {
		t.Fatalf("pass value = %q", got)
	}
	state, err := os.ReadFile(f.env.Paths.State())
	if err != nil {
		t.Fatalf("state file: %v", err)
	}
	if bytes.Contains(state, []byte("pd-secret-123")) {
		t.Fatalf("state file holds the secret: %s", state)
	}
	if !strings.Contains(string(state), "pagerduty-api-key") {
		t.Fatalf("state file missing entry: %s", state)
	}
}

func TestSyncConflictSkipKeepsOldValue(t *testing.T) {
	f := newCliFixture(t, "s\n")
	f.writeMapping(t, pagerduty)
	f.env.SetOp(t, pagerduty.Ref, "new-value")
	f.env.SetOpSignedIn(t, true)
	f.env.Insert(t, "pagerduty-api-key", "old-value")

	if err := f.run(f.app.syncCommand()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := f.env.Show(t, "pagerduty-api-key"); got != "old-value" {
		t.Fatalf("pass value = %q, want old-value", got)
	}
	if strings.Contains(f.out.String(), "new-value") {
		t.Fatalf("output leaked the secret:\n%s", f.out.String())
	}
}

func TestSyncConflictOverwrite(t *testing.T) {
	f := newCliFixture(t, "o\n")
	f.writeMapping(t, pagerduty)
	f.env.SetOp(t, pagerduty.Ref, "new-value")
	f.env.SetOpSignedIn(t, true)
	f.env.Insert(t, "pagerduty-api-key", "old-value")

	if err := f.run(f.app.syncCommand()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := f.env.Show(t, "pagerduty-api-key"); got != "new-value" {
		t.Fatalf("pass value = %q, want new-value", got)
	}
}

func TestSyncConflictRevealThenSkip(t *testing.T) {
	f := newCliFixture(t, "v\ns\n")
	f.writeMapping(t, pagerduty)
	f.env.SetOp(t, pagerduty.Ref, "new-value")
	f.env.SetOpSignedIn(t, true)
	f.env.Insert(t, "pagerduty-api-key", "old-value")

	if err := f.run(f.app.syncCommand()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	out := f.out.String()
	if !strings.Contains(out, "new-value") || !strings.Contains(out, "old-value") {
		t.Fatalf("reveal did not print both values:\n%s", out)
	}
	if strings.Count(out, "new-value") != 1 {
		t.Fatalf("secret printed more than on reveal:\n%s", out)
	}
	if got := f.env.Show(t, "pagerduty-api-key"); got != "old-value" {
		t.Fatalf("pass value = %q, want old-value", got)
	}
}

func TestSyncConflictEOFQuits(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	f.env.SetOp(t, pagerduty.Ref, "new-value")
	f.env.SetOpSignedIn(t, true)
	f.env.Insert(t, "pagerduty-api-key", "old-value")

	err := f.run(f.app.syncCommand())
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit code = %d, err %v", code, err)
	}
	if got := f.env.Show(t, "pagerduty-api-key"); got != "old-value" {
		t.Fatalf("pass value = %q, want old-value", got)
	}
}

func TestSyncFailingRefExits2(t *testing.T) {
	broken := mapping.Secret{Pass: "missing-entry", Ref: "op://Private/Missing/field", Env: "MISSING"}
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty, broken)
	f.env.SetOp(t, pagerduty.Ref, "pd-secret-123")
	f.env.SetOpSignedIn(t, true)

	err := f.run(f.app.syncCommand())
	if code := exitCode(t, err); code != 2 {
		t.Fatalf("exit code = %d, err %v", code, err)
	}
	if !strings.Contains(f.out.String(), "✕ missing-entry") {
		t.Fatalf("output missing failed line:\n%s", f.out.String())
	}
	if got := f.env.Show(t, "pagerduty-api-key"); got != "pd-secret-123" {
		t.Fatalf("other entry not synced: %q", got)
	}
}

func TestSyncSkipConflictsExits1(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	f.env.SetOp(t, pagerduty.Ref, "new-value")
	f.env.SetOpSignedIn(t, true)
	f.env.Insert(t, "pagerduty-api-key", "old-value")

	err := f.run(f.app.syncCommand(), "--skip-conflicts")
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, err %v", code, err)
	}
	if got := f.env.Show(t, "pagerduty-api-key"); got != "old-value" {
		t.Fatalf("pass value = %q, want old-value", got)
	}
}

func TestSyncMissingMappingTellsInit(t *testing.T) {
	f := newCliFixture(t, "")
	err := f.run(f.app.syncCommand())
	if err == nil || !strings.Contains(err.Error(), "pvault mapping init") {
		t.Fatalf("err = %v", err)
	}
}

func TestSyncSignedOutIncludesHint(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	f.env.SetOpSignedIn(t, false)
	err := f.run(f.app.syncCommand())
	if err == nil || !strings.Contains(err.Error(), "Integrate with 1Password CLI") {
		t.Fatalf("err = %v", err)
	}
}
