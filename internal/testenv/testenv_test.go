package testenv

import (
	"os/exec"
	"strings"
	"testing"
)

func TestStoreRoundTripsExactBytes(t *testing.T) {
	env := New(t).WithStore(t)
	env.Insert(t, "a/b", "line1\nline2\n")
	if got := env.Show(t, "a/b"); got != "line1\nline2\n" {
		t.Fatalf("got %q", got)
	}
}

func TestFakeOpReadsFixtureAndLogs(t *testing.T) {
	env := New(t)
	env.SetOp(t, "op://Private/Item/field", "value")
	out, err := exec.Command("op", "read", "-n", "op://Private/Item/field").Output()
	if err != nil || string(out) != "value" {
		t.Fatalf("out %q err %v", out, err)
	}
	if _, err := exec.Command("op", "read", "-n", "op://Private/Missing/f").Output(); err == nil {
		t.Fatal("missing reference should fail")
	}
	if log := env.Log(t, "op"); len(log) != 2 || !strings.HasPrefix(log[0], "read -n op://") {
		t.Fatalf("log = %q", log)
	}
}

func TestFakeBrewTracksInstallState(t *testing.T) {
	env := New(t)
	if exec.Command("brew", "list", "--versions", "pass").Run() == nil {
		t.Fatal("pass should start uninstalled")
	}
	if err := exec.Command("brew", "install", "pass").Run(); err != nil {
		t.Fatal(err)
	}
	if !env.BrewInstalled("pass") {
		t.Fatal("install did not record pass")
	}
	env.SetBrewUses(t, "gnupg", "pass")
	out, _ := exec.Command("brew", "uses", "--installed", "gnupg").Output()
	if strings.TrimSpace(string(out)) != "pass" {
		t.Fatalf("uses = %q", out)
	}
}
