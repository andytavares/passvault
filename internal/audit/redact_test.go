package audit

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRedactReplacesSecretText(t *testing.T) {
	known := map[string]string{"LINEAR_API_KEY": "lin_test_value_123456", "PIN": "4711"}
	cases := []struct {
		name, text, want string
	}{
		{"known value", "curl -H 'x: lin_test_value_123456' api", "curl -H 'x: [LINEAR_API_KEY]' api"},
		{"known value under 8 bytes", "pin 4711 ok", "pin 4711 ok"},
		{"github token", "git push https://ghp_A1b2C3d4E5f6G7h8I9j0@github.com", "git push https://[redacted]@github.com"},
		{"aws key", "aws --key AKIAIOSFODNN7EXAMPLE1", "aws --key [redacted]"},
		{"slack token", "post xoxb-123456789012-abcdefGHIJ", "post [redacted]"},
		{"word containing sk-", "task-runner-abcdefghijklmnop", "task-runner-abcdefghijklmnop"},
		{"bearer header", "curl -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.e30.x' api", "curl -H 'Authorization: Bearer [redacted]' api"},
		{"lower-case basic header", "authorization: Basic dXNlcjpwYXNz", "authorization: [redacted]"},
		{"flag with space", "deploy --token abcdef1234567890 now", "deploy --token [redacted] now"},
		{"flag with equals", "login --password=hunter2hunter2", "login --password=[redacted]"},
		{"assignment", "env GITHUB_TOKEN=abcdef123 make", "env GITHUB_TOKEN=[redacted] make"},
		{"assignment of other name", "env GOFLAGS=-race make", "env GOFLAGS=-race make"},
		{"no secret", "pvault get github --field user", "pvault get github --field user"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Redact(tc.text, known); got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestRedactReplacesLongerKnownValueFirst(t *testing.T) {
	known := map[string]string{"SHORT_KEY": "abcdefgh", "LONG_KEY": "abcdefgh-ijklmnop"}
	if got := Redact("use abcdefgh-ijklmnop", known); got != "use [LONG_KEY]" {
		t.Errorf("Redact = %q, want %q", got, "use [LONG_KEY]")
	}
}

func TestRedactArgvReplacesElementAfterFlag(t *testing.T) {
	got := RedactArgv([]string{"curl", "--token", "abcdef1234567890"}, nil)
	if !slices.Equal(got, []string{"curl", "--token", "[redacted]"}) {
		t.Errorf("RedactArgv = %q", got)
	}
}

func TestRecordRedactsBeforeCuttingArgs(t *testing.T) {
	recorder := newRecorder(t, "github")
	recorder.Known = func() map[string]string { return map[string]string{"LINEAR_API_KEY": "lin_test_value_123456"} }
	longArg := strings.Repeat("x", 109) + "=ghp_Z9y8X7w6V5u4T3s2"
	recorder.Callers = func() []Process {
		return []Process{{PID: 4242, Comm: "zsh", Argv: []string{"run", "lin_test_value_123456", "ghp_A1b2C3d4E5f6G7h8I9j0", longArg}}}
	}
	if err := recorder.Record("github", ActionUse, "key lin_test_value_123456"); err != nil {
		t.Fatal(err)
	}
	line := string(rawLines(t, recorder.Path)[0])
	for _, secret := range []string{"lin_test_value", "ghp_A1b2C3", "ghp_Z9y8X7"} {
		if strings.Contains(line, secret) {
			t.Errorf("event line contains %q: %s", secret, line)
		}
	}
	events, err := Read(recorder.Path)
	if err != nil {
		t.Fatal(err)
	}
	argv := events[0].Callers[0].Argv
	want := []string{"run", "[LINEAR_API_KEY]", "[redacted]", strings.Repeat("x", 109) + "=[redacted]"}
	if !slices.Equal(argv, want) || events[0].Detail != "key [LINEAR_API_KEY]" {
		t.Errorf("argv = %q, detail = %q; want %q and key [LINEAR_API_KEY]", argv, events[0].Detail, want)
	}
}

func TestRecordCutsLongArgToMaxBytes(t *testing.T) {
	recorder := newRecorder(t, "github")
	recorder.Callers = func() []Process {
		return []Process{{PID: 4242, Comm: "zsh", Argv: []string{strings.Repeat("y", 130)}}}
	}
	got := recordOne(t, recorder)
	if got.Callers[0].Argv[0] != strings.Repeat("y", maxArgBytes) {
		t.Errorf("argv = %q, want %d bytes", got.Callers[0].Argv, maxArgBytes)
	}
}

// TestMain turns the test binary into a sleeping child when asked, because
// macOS hides the environment of Apple's own binaries such as /bin/sleep.
func TestMain(m *testing.M) {
	if os.Getenv("PVAULT_HELPER_SLEEP") == "1" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestHoldersFindsChildWithEnvName(t *testing.T) {
	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), "PVAULT_HELPER_SLEEP=1", "PVAULT_HOLDER_TEST=secret-holder-value")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	holders, err := Holders("PVAULT_HOLDER_TEST")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(holders, func(p Process) bool { return p.PID == child.Process.Pid }) {
		t.Errorf("holders = %+v, want child pid %d", holders, child.Process.Pid)
	}
	if strings.Contains(fmt.Sprintf("%+v", holders), "secret-holder-value") {
		t.Errorf("holders contain the env value: %+v", holders)
	}
}

func TestHoldersOfUnusedNameIsEmpty(t *testing.T) {
	holders, err := Holders("PVAULT_NO_PROCESS_HAS_THIS")
	if err != nil || len(holders) != 0 {
		t.Fatalf("Holders = %+v, %v; want none", holders, err)
	}
}
