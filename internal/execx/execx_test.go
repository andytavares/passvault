package execx

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunPassesStdinAndCapturesStdout(t *testing.T) {
	result, err := OS{}.Run(context.Background(), Cmd{Name: "cat", Stdin: []byte("a\nb")})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(result.Stdout); got != "a\nb" {
		t.Fatalf("stdout = %q, want %q", got, "a\nb")
	}
}

func TestRunReturnsExitErrorWithStderr(t *testing.T) {
	_, err := OS{}.Run(context.Background(), Cmd{Name: "sh", Args: []string{"-c", "echo boom >&2; exit 3"}})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want *ExitError", err)
	}
	if exitErr.Code != 3 || !strings.Contains(exitErr.Error(), "boom") {
		t.Fatalf("got code %d, message %q", exitErr.Code, exitErr.Error())
	}
}

func TestRunAddsEnv(t *testing.T) {
	result, err := OS{}.Run(context.Background(), Cmd{Name: "sh", Args: []string{"-c", "printf %s \"$PV_X\""}, Env: []string{"PV_X=set"}})
	if err != nil || string(result.Stdout) != "set" {
		t.Fatalf("stdout = %q, err = %v", result.Stdout, err)
	}
}

func TestRecorderStringNeverIncludesStdin(t *testing.T) {
	recorder := &Recorder{Inner: OS{}}
	_, _ = recorder.Run(context.Background(), Cmd{Name: "cat", Args: []string{"-u"}, Stdin: []byte("s3cret")})
	argv := recorder.Argv()
	if len(argv) != 1 || argv[0] != "cat -u" {
		t.Fatalf("argv = %q", argv)
	}
}
