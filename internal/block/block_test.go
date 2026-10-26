package block

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const userText = "export A=1\n# keep me\n"

func TestUpsertAppendsAndKeepsOtherBytes(t *testing.T) {
	got, err := Upsert(userText, "eval x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, userText) || !strings.HasSuffix(got, Render("eval x")) {
		t.Fatalf("got %q", got)
	}
}

func TestUpsertAddsNewlineWhenFileLacksOne(t *testing.T) {
	got, _ := Upsert("export A=1", "x")
	if !strings.HasPrefix(got, "export A=1\n"+Begin) {
		t.Fatalf("got %q", got)
	}
}

func TestUpsertTwiceIsByteIdentical(t *testing.T) {
	once, _ := Upsert(userText, "eval x")
	twice, _ := Upsert(once, "eval x")
	if once != twice {
		t.Fatalf("not idempotent:\n%q\n%q", once, twice)
	}
}

func TestUpsertReplacesBodyInPlace(t *testing.T) {
	content := "before\n" + Render("old") + "after\n"
	got, _ := Upsert(content, "new")
	if got != "before\n"+Render("new")+"after\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRemoveRestoresOriginal(t *testing.T) {
	with, _ := Upsert(userText, "eval x")
	got, removed, err := Remove(with)
	if err != nil || !removed || got != userText {
		t.Fatalf("got %q removed %v err %v", got, removed, err)
	}
}

func TestBrokenMarkersAreErrors(t *testing.T) {
	for _, content := range []string{Begin + "\nx\n", "x\n" + End + "\n"} {
		if _, err := Upsert(content, "y"); err == nil {
			t.Errorf("no error for %q", content)
		}
	}
}

func TestUpsertFileBacksUpOnlyFirstEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshrc")
	if err := os.WriteFile(path, []byte(userText), 0o640); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)
	backup, err := UpsertFile(path, "eval x", now)
	if err != nil || backup != path+".pvault-20261025T130000" {
		t.Fatalf("backup %q err %v", backup, err)
	}
	if saved, _ := os.ReadFile(backup); string(saved) != userText {
		t.Fatalf("backup content %q", saved)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode changed to %v", info.Mode().Perm())
	}
	second, err := UpsertFile(path, "eval y", now)
	if err != nil || second != "" {
		t.Fatalf("second edit made backup %q err %v", second, err)
	}
}

func TestRemoveFileOnMissingFileIsNoop(t *testing.T) {
	changed, err := RemoveFile(filepath.Join(t.TempDir(), "absent"))
	if err != nil || changed {
		t.Fatalf("changed %v err %v", changed, err)
	}
}
