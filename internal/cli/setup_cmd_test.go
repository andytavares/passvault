package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupFreshHomePrintsStepsAndWritesManifest(t *testing.T) {
	f := newCliFixture(t, "y\n\n\n")
	if err := f.run(f.app.setupCommand()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	out := f.out.String()
	for _, want := range []string{"pvault setup", "+ pass", "+ zshrc block", "Manifest: " + f.env.Paths.Manifest()} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(f.env.Paths.Manifest()); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if got := len(brewLog(f, t, "install")); got != 4 {
		t.Fatalf("brew installs = %d", got)
	}
}

func TestSetupSecondRunInstallsNothing(t *testing.T) {
	f := newCliFixture(t, "y\n\n\n")
	if err := f.run(f.app.setupCommand()); err != nil {
		t.Fatalf("first setup: %v", err)
	}
	f.out.Reset()
	if err := f.run(f.app.setupCommand()); err != nil {
		t.Fatalf("second setup: %v", err)
	}
	if got := len(brewLog(f, t, "install")); got != 4 {
		t.Fatalf("brew installs after rerun = %d", got)
	}
	if strings.Contains(f.out.String(), "\n  + ") {
		t.Fatalf("second run applied steps:\n%s", f.out.String())
	}
}

func TestSetupDryRunLeavesHomeUntouched(t *testing.T) {
	f := newCliFixture(t, "")
	before := hashHome(t, f)
	if err := f.run(f.app.setupCommand(), "--dry-run"); err != nil {
		t.Fatalf("setup --dry-run: %v", err)
	}
	if after := hashHome(t, f); after != before {
		t.Fatal("dry run changed HOME")
	}
	if _, err := os.Stat(f.env.Paths.Manifest()); !os.IsNotExist(err) {
		t.Fatal("manifest written on dry run")
	}
}

func TestUninstallDryRunPrintsPlanAndChangesNothing(t *testing.T) {
	f := newCliFixture(t, "y\n\n\n")
	if err := f.run(f.app.setupCommand()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	before := hashHome(t, f)
	if err := f.run(f.app.uninstallCommand(), "--dry-run"); err != nil {
		t.Fatalf("uninstall --dry-run: %v", err)
	}
	out := f.out.String()
	for _, want := range []string{"Will remove", "Will keep"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if after := hashHome(t, f); after != before {
		t.Fatal("dry run changed HOME")
	}
}

func TestUninstallYesRemovesBlockAndPackages(t *testing.T) {
	f := newCliFixture(t, "y\n\n\n")
	if err := f.run(f.app.setupCommand()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := f.run(f.app.uninstallCommand(), "--yes"); err != nil {
		t.Fatalf("uninstall --yes: %v", err)
	}
	data, err := os.ReadFile(f.env.Paths.Zshrc())
	if err != nil {
		t.Fatalf("zshrc: %v", err)
	}
	if strings.Contains(string(data), "pvault") {
		t.Fatalf("zshrc block kept:\n%s", data)
	}
	if f.env.BrewInstalled("pinentry-mac") {
		t.Fatal("pinentry-mac kept")
	}
	if _, err := os.Stat(f.env.Paths.Manifest()); !os.IsNotExist(err) {
		t.Fatal("manifest kept")
	}
}

func TestUninstallPurgeVaultWithWrongPathKeepsStore(t *testing.T) {
	f := newCliFixture(t, "y\n\n\n")
	if err := f.run(f.app.setupCommand()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f.app.in = strings.NewReader("/wrong\n")
	err := f.run(f.app.uninstallCommand(), "--yes", "--purge-vault")
	if err == nil {
		t.Fatal("no error")
	}
	if _, statErr := os.Stat(f.env.Paths.StoreDir); statErr != nil {
		t.Fatal("store deleted")
	}
}

// brewLog returns the fake brew log lines that start with verb.
func brewLog(f *cliFixture, t *testing.T, verb string) []string {
	var out []string
	for _, line := range f.env.Log(t, "brew") {
		if line == verb || strings.HasPrefix(line, verb+" ") {
			out = append(out, line)
		}
	}
	return out
}

// hashHome digests the HOME tree, skipping fake tool logs and gpg's own
// keybox housekeeping, which are harness and gpg writes, not pvault writes.
func hashHome(t *testing.T, f *cliFixture) string {
	t.Helper()
	digest := sha256.New()
	err := filepath.Walk(f.env.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(f.env.Root, path)
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
