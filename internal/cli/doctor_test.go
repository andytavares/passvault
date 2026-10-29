package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

// healthyEnv runs setup so doctor has a working store, key and mapping.
func healthyEnv(t *testing.T) *cliFixture {
	t.Helper()
	f := newCliFixture(t, "y\n\n\n")
	if err := f.run(f.app.setupCommand()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f.out.Reset()
	return f
}

func TestDoctorHealthyEnvExitsNil(t *testing.T) {
	f := healthyEnv(t)
	if err := f.run(f.app.doctorCommand()); err != nil {
		t.Fatalf("doctor: %v\n%s", err, f.out.String())
	}
	if strings.Contains(f.out.String(), "✕") {
		t.Fatalf("healthy env has a failure:\n%s", f.out.String())
	}
}

func TestDoctorBadMappingExits1(t *testing.T) {
	f := healthyEnv(t)
	bad := "version = 1\n\n[[secret]]\npass = \"x\"\nref = \"not-op\"\n"
	if err := os.WriteFile(f.env.Paths.Mapping(), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	err := f.run(f.app.doctorCommand())
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, err %v", code, err)
	}
	if !strings.Contains(f.out.String(), "op://") {
		t.Fatalf("output missing the problem:\n%s", f.out.String())
	}
}

func TestDoctorFlagsPassOutsideBlockAndGlobalEntries(t *testing.T) {
	f := healthyEnv(t)
	zshrc, err := os.ReadFile(f.env.Paths.Zshrc())
	if err != nil {
		t.Fatal(err)
	}
	extra := string(zshrc) + "export TOKEN=$(pass work/token)\n"
	if err := os.WriteFile(f.env.Paths.Zshrc(), []byte(extra), 0o600); err != nil {
		t.Fatal(err)
	}
	f.writeMapping(t, mapping.Secret{Pass: "work/token", Ref: "op://Private/T/token", Env: "TOKEN"})
	err = f.run(f.app.doctorCommand())
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit code = %d, err %v\n%s", code, err, f.out.String())
	}
	out := f.out.String()
	if !strings.Contains(out, "runs pass outside the managed block") {
		t.Fatalf("output missing zshrc warning:\n%s", out)
	}
	if !strings.Contains(out, "TOKEN") || !strings.Contains(out, "exported to every child process") {
		t.Fatalf("output missing global entry note:\n%s", out)
	}
}

func TestDoctorMissingStoreFails(t *testing.T) {
	f := newCliFixture(t, "")
	if err := os.WriteFile(filepath.Join(f.env.Root, ".zshrc"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	err := f.run(f.app.doctorCommand())
	if code := exitCode(t, err); code != 1 {
		t.Fatalf("exit code = %d, err %v", code, err)
	}
	if !strings.Contains(f.out.String(), "✕") {
		t.Fatalf("output missing failure:\n%s", f.out.String())
	}
}

func TestDoctorSkipsGlobalWarningForDisabledEntry(t *testing.T) {
	f := healthyEnv(t)
	off := false
	f.writeMapping(t, mapping.Secret{Pass: "work/token", Ref: "op://Private/T/token", Env: "TOKEN", Enabled: &off})
	if err := f.run(f.app.doctorCommand()); err != nil {
		t.Fatalf("doctor: %v\n%s", err, f.out.String())
	}
	if strings.Contains(f.out.String(), "exported to every child process") {
		t.Fatalf("disabled entry warned:\n%s", f.out.String())
	}
}
