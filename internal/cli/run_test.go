package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/execx"
)

// captureRunner runs the child with stdout redirected to a file, since
// Terminal mode attaches the child's output to the process streams.
type captureRunner struct {
	inner   execx.Runner
	outPath string
}

func (r captureRunner) Run(ctx context.Context, c execx.Cmd) (execx.Result, error) {
	if !c.Terminal {
		return r.inner.Run(ctx, c)
	}
	proc := exec.CommandContext(ctx, c.Name, c.Args...)
	proc.Env = append(os.Environ(), c.Env...)
	out, err := os.Create(r.outPath)
	if err != nil {
		return execx.Result{}, err
	}
	defer out.Close()
	proc.Stdout, proc.Stderr = out, out
	if err := proc.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return execx.Result{}, &execx.ExitError{Cmd: c.String(), Code: exitErr.ExitCode()}
		}
		return execx.Result{}, err
	}
	return execx.Result{}, nil
}

func newRunFixture(t *testing.T) (*cliFixture, string) {
	t.Helper()
	f := newCliFixture(t, "")
	outPath := f.env.Root + "/run.out"
	f.app.runner = captureRunner{inner: f.app.runner, outPath: outPath}
	return f, outPath
}

func TestRunSetsVariableForChild(t *testing.T) {
	f, outPath := newRunFixture(t)
	f.writeMapping(t, pagerduty)
	f.env.Insert(t, "pagerduty-api-key", "run-value")

	err := f.run(f.app.runCommand(), "--entry", "pagerduty-api-key", "--",
		"sh", "-c", `printf %s "$PAGERDUTY_API_KEY"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "run-value" {
		t.Fatalf("child printed %q", data)
	}
}

func TestRunPropagatesExitCode(t *testing.T) {
	f, _ := newRunFixture(t)
	f.writeMapping(t, pagerduty)
	f.env.Insert(t, "pagerduty-api-key", "run-value")

	err := f.run(f.app.runCommand(), "--entry", "pagerduty-api-key", "--", "sh", "-c", "exit 7")
	if code := exitCode(t, err); code != 7 {
		t.Fatalf("exit code = %d, err %v", code, err)
	}
}

func TestRunRequiresCommand(t *testing.T) {
	f, _ := newRunFixture(t)
	f.writeMapping(t, pagerduty)
	err := f.run(f.app.runCommand(), "--entry", "pagerduty-api-key")
	if err == nil || !strings.Contains(err.Error(), "CMD") {
		t.Fatalf("err = %v", err)
	}
}
