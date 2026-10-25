package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootWithoutTerminalPrintsHelpInsteadOfOpeningTUI(t *testing.T) {
	fixture := newCliFixture(t, "")
	root := &cobra.Command{Use: "pvault"}
	registerTUICommand(root, fixture.app)
	root.AddCommand(&cobra.Command{Use: "sub"})
	root.SetOut(fixture.out)
	root.SetArgs(nil)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fixture.out.String(), "Available Commands") {
		t.Fatalf("expected help text, got %q", fixture.out.String())
	}
}

func TestIsTerminalRejectsPipesAndFiles(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if isTerminal(file) {
		t.Fatal("regular file reported as terminal")
	}
	if isTerminal(&bytes.Buffer{}) {
		t.Fatal("buffer reported as terminal")
	}
}

func TestEditorCommandHonoursEDITOR(t *testing.T) {
	t.Setenv("EDITOR", "nano")
	if got := editorCommand("/x/mapping.toml").Args; len(got) != 2 || got[0] != "nano" || got[1] != "/x/mapping.toml" {
		t.Fatalf("args = %q", got)
	}
	t.Setenv("EDITOR", "")
	if got := editorCommand("/x").Args[0]; got != "vi" {
		t.Fatalf("fallback editor = %q", got)
	}
}
