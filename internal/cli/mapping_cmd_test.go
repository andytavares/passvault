package cli

import (
	"os"
	"strings"
	"testing"
)

func TestMappingInitIsIdempotent(t *testing.T) {
	f := newCliFixture(t, "")
	if err := f.run(f.app.mappingCommand(), "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(f.out.String(), "Created") {
		t.Fatalf("output:\n%s", f.out.String())
	}
	data, err := os.ReadFile(f.env.Paths.Mapping())
	if err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	if err := f.run(f.app.mappingCommand(), "init"); err != nil {
		t.Fatalf("second init: %v", err)
	}
	if !strings.Contains(f.out.String(), "already exists") {
		t.Fatalf("output:\n%s", f.out.String())
	}
	again, err := os.ReadFile(f.env.Paths.Mapping())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(again) {
		t.Fatal("second init changed the file")
	}
}

func TestMappingPathPrintsPath(t *testing.T) {
	f := newCliFixture(t, "")
	if err := f.run(f.app.mappingCommand(), "path"); err != nil {
		t.Fatalf("path: %v", err)
	}
	if strings.TrimSpace(f.out.String()) != f.env.Paths.Mapping() {
		t.Fatalf("output = %q", f.out.String())
	}
}

func TestMappingValidateBadFileExits1(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	content, err := os.ReadFile(f.env.Paths.Mapping())
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(content), "op://Private/PagerDuty/credential", "not-a-ref", 1)
	if err := os.WriteFile(f.env.Paths.Mapping(), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}

	err = f.run(f.app.mappingCommand(), "validate")
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, err %v", code, err)
	}
	if !strings.Contains(f.out.String(), "ref must start with op://") {
		t.Fatalf("missing error line:\n%s", f.out.String())
	}
}

func TestMappingValidateOK(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	if err := f.run(f.app.mappingCommand(), "validate"); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !strings.Contains(f.out.String(), "OK, 1 entries") {
		t.Fatalf("output:\n%s", f.out.String())
	}
}

func TestMappingAddAppendsAndRevalidates(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	err := f.run(f.app.mappingCommand(), "add",
		"--pass", "work/github-token", "--ref", "op://Employee/GitHub PAT/token",
		"--env", "GITHUB_TOKEN", "--scope", "command", "--command", "gh")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	data, err := os.ReadFile(f.env.Paths.Mapping())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`pass = "work/github-token"`, `env = "GITHUB_TOKEN"`, `scope = "command"`, `commands = ["gh"]`} {
		if !strings.Contains(text, want) {
			t.Fatalf("mapping missing %s:\n%s", want, text)
		}
	}
	f.out.Reset()
	if err := f.run(f.app.mappingCommand(), "validate"); err != nil {
		t.Fatalf("re-validate: %v", err)
	}
	if !strings.Contains(f.out.String(), "OK, 2 entries") {
		t.Fatalf("output:\n%s", f.out.String())
	}
}

func TestMappingAddRejectsDuplicate(t *testing.T) {
	f := newCliFixture(t, "")
	f.writeMapping(t, pagerduty)
	err := f.run(f.app.mappingCommand(), "add",
		"--pass", "pagerduty-api-key", "--ref", "op://Private/Other/field")
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err = %v", err)
	}
}
