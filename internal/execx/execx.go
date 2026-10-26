// Package execx runs external programs (pass, gpg, op, git, brew) behind an
// interface so tests can record every argv and assert no secret is in it.
package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Cmd describes one program run. Secrets go in Stdin, never in Args.
type Cmd struct {
	Name  string
	Args  []string
	Stdin []byte
	Env   []string // extra KEY=VALUE pairs added to the current environment
	Dir   string
	// Terminal connects the process to this process's stdin, stdout and
	// stderr, for pinentry, $EDITOR and other interactive programs.
	Terminal bool
}

// String returns the argv joined by spaces. It never includes Stdin.
func (c Cmd) String() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

// Result holds the captured output of a finished program.
type Result struct {
	Stdout []byte
	Stderr []byte
}

// ExitError reports a program that ran and exited non-zero.
type ExitError struct {
	Cmd    string
	Code   int
	Stderr string
}

func (e *ExitError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		return fmt.Sprintf("%s: exit status %d", e.Cmd, e.Code)
	}
	return fmt.Sprintf("%s: exit status %d: %s", e.Cmd, e.Code, msg)
}

// Runner runs a Cmd. A non-zero exit returns *ExitError with the output.
type Runner interface {
	Run(ctx context.Context, cmd Cmd) (Result, error)
}

// OS runs programs with os/exec.
type OS struct{}

// Run implements Runner.
func (OS) Run(ctx context.Context, c Cmd) (Result, error) {
	proc := exec.CommandContext(ctx, c.Name, c.Args...)
	proc.Dir = c.Dir
	proc.Env = append(os.Environ(), c.Env...)
	var stdout, stderr bytes.Buffer
	if c.Terminal {
		proc.Stdin, proc.Stdout, proc.Stderr = os.Stdin, os.Stdout, os.Stderr
	} else {
		proc.Stdin = bytes.NewReader(c.Stdin)
		proc.Stdout, proc.Stderr = &stdout, &stderr
	}
	err := proc.Run()
	result := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return result, &ExitError{Cmd: c.String(), Code: exitErr.ExitCode(), Stderr: stderr.String()}
	}
	return result, err
}

// Recorder wraps a Runner and keeps every Cmd it ran, for test assertions.
type Recorder struct {
	Inner Runner
	Calls []Cmd
}

// Run implements Runner.
func (r *Recorder) Run(ctx context.Context, c Cmd) (Result, error) {
	r.Calls = append(r.Calls, c)
	return r.Inner.Run(ctx, c)
}

// Argv returns every recorded call as its argv string.
func (r *Recorder) Argv() []string {
	out := make([]string, len(r.Calls))
	for i, c := range r.Calls {
		out[i] = c.String()
	}
	return out
}
