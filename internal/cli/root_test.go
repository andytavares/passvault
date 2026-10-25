package cli

import (
	"bytes"
	"testing"
)

func TestVersionPrintsBuildVersion(t *testing.T) {
	Version = "v9.9.9"
	t.Cleanup(func() { Version = "dev" })
	root := NewRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "pvault v9.9.9\n" {
		t.Fatalf("got %q", out.String())
	}
}
