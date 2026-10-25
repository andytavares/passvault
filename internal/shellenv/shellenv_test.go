package shellenv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

func readValues(values map[string][]byte) ReadSecret {
	return func(_ context.Context, name string) ([]byte, error) {
		value, ok := values[name]
		if !ok {
			return nil, fmt.Errorf("no such entry: %s", name)
		}
		return value, nil
	}
}

func runZsh(t *testing.T, script string, env ...string) string {
	t.Helper()
	cmd := exec.Command("zsh", "-c", script)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zsh -c: %v\n%s", err, out)
	}
	return string(out)
}

func TestRenderQuoteInjection(t *testing.T) {
	temp := t.TempDir()
	marker := filepath.Join(temp, "x")
	value := "a'b$(touch " + marker + ")"
	m := mapping.Mapping{Secrets: []mapping.Secret{
		{Pass: "test", Env: "PV_TEST", Scope: mapping.ScopeGlobal},
	}}
	script, warnings := RenderScript(context.Background(), m, readValues(map[string][]byte{"test": []byte(value)}))
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	out := runZsh(t, script+`; printf %s "$PV_TEST"`)
	if out != value {
		t.Fatalf("got %q, want %q", out, value)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("value ran code: marker file exists")
	}
}

func TestRenderQuoteNewlines(t *testing.T) {
	value := "line one\nline two\n"
	m := mapping.Mapping{Secrets: []mapping.Secret{
		{Pass: "test", Env: "PV_TEST", Scope: mapping.ScopeGlobal},
	}}
	script, _ := RenderScript(context.Background(), m, readValues(map[string][]byte{"test": []byte(value)}))
	out := runZsh(t, script+`; printf %s "$PV_TEST"`)
	if out != value {
		t.Fatalf("got %q, want %q", out, value)
	}
}

func TestRenderReadErrorWarnsAndSkips(t *testing.T) {
	m := mapping.Mapping{Secrets: []mapping.Secret{
		{Pass: "broken", Env: "PV_BROKEN", Scope: mapping.ScopeGlobal},
		{Pass: "fine", Env: "PV_FINE", Scope: mapping.ScopeGlobal},
	}}
	read := func(_ context.Context, name string) ([]byte, error) {
		if name == "broken" {
			return nil, errors.New("decrypt failed")
		}
		return []byte("ok"), nil
	}
	script, warnings := RenderScript(context.Background(), m, read)
	if len(warnings) != 1 || warnings[0].Pass != "broken" {
		t.Fatalf("warnings: %v", warnings)
	}
	if !strings.Contains(script, "export PV_FINE='ok'") {
		t.Fatalf("script missing other export:\n%s", script)
	}
	if strings.Contains(script, "PV_BROKEN") {
		t.Fatalf("script contains failed entry:\n%s", script)
	}
}

func TestRenderNULWarnsAndSkips(t *testing.T) {
	m := mapping.Mapping{Secrets: []mapping.Secret{
		{Pass: "nul", Env: "PV_NUL", Scope: mapping.ScopeGlobal},
		{Pass: "fine", Env: "PV_FINE", Scope: mapping.ScopeGlobal},
	}}
	read := readValues(map[string][]byte{"nul": []byte("a\x00b"), "fine": []byte("ok")})
	script, warnings := RenderScript(context.Background(), m, read)
	if len(warnings) != 1 || warnings[0].Pass != "nul" {
		t.Fatalf("warnings: %v", warnings)
	}
	if strings.Contains(script, "PV_NUL") {
		t.Fatalf("script contains NUL entry:\n%s", script)
	}
}

func TestRenderCommandScope(t *testing.T) {
	m := mapping.Mapping{Secrets: []mapping.Secret{
		{Pass: "work/github-token", Env: "GITHUB_TOKEN", Scope: mapping.ScopeCommand, Commands: []string{"gh"}},
	}}
	read := func(_ context.Context, name string) ([]byte, error) {
		t.Fatalf("read called for command-scope entry %s", name)
		return nil, nil
	}
	script, warnings := RenderScript(context.Background(), m, read)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}

	bin := t.TempDir()
	pass := "#!/bin/sh\nif [ \"$1\" = show ]; then printf %s 'token-123'; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "pass"), []byte(pass), 0o755); err != nil {
		t.Fatal(err)
	}
	gh := "#!/bin/sh\nprintf %s \"$GITHUB_TOKEN\"\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o755); err != nil {
		t.Fatal(err)
	}

	out := runZsh(t, script+"\ngh\nprintf %s \"${GITHUB_TOKEN-unset}\"\n", "PATH="+bin+":"+os.Getenv("PATH"))
	if out != "token-123unset" {
		t.Fatalf("got %q, want %q", out, "token-123unset")
	}
}

func TestRenderCommandScopeMerges(t *testing.T) {
	m := mapping.Mapping{Secrets: []mapping.Secret{
		{Pass: "a", Env: "A", Scope: mapping.ScopeCommand, Commands: []string{"gh"}},
		{Pass: "b", Env: "B", Scope: mapping.ScopeCommand, Commands: []string{"gh"}},
	}}
	script, _ := RenderScript(context.Background(), m, readValues(nil))
	want := "gh() { A=\"$(command pass show 'a')\" B=\"$(command pass show 'b')\" command gh \"$@\"; }"
	if strings.Count(script, "gh()") != 1 || !strings.Contains(script, want) {
		t.Fatalf("script:\n%s", script)
	}
}

func TestRenderOrderingAndEmpty(t *testing.T) {
	m := mapping.Mapping{Secrets: []mapping.Secret{
		{Pass: "z", Env: "ZED", Scope: mapping.ScopeGlobal},
		{Pass: "a", Env: "AYE", Scope: mapping.ScopeGlobal},
		{Pass: "c1", Env: "C1", Scope: mapping.ScopeCommand, Commands: []string{"zed"}},
		{Pass: "c2", Env: "C2", Scope: mapping.ScopeCommand, Commands: []string{"alpha"}},
		{Pass: "skip", Scope: mapping.ScopeNone},
		{Pass: "noenv", Scope: mapping.ScopeGlobal},
	}}
	read := readValues(map[string][]byte{"z": []byte("1"), "a": []byte("2")})
	script, _ := RenderScript(context.Background(), m, read)
	want := "export ZED='1'\nexport AYE='2'\n" +
		"alpha() { C2=\"$(command pass show 'c2')\" command alpha \"$@\"; }\n" +
		"zed() { C1=\"$(command pass show 'c1')\" command zed \"$@\"; }\n"
	if script != want {
		t.Fatalf("got %q, want %q", script, want)
	}
	empty, _ := RenderScript(context.Background(), mapping.Mapping{}, readValues(nil))
	if empty != "" {
		t.Fatalf("empty mapping: got %q", empty)
	}
}

func TestSnippetQuotesPath(t *testing.T) {
	got := Snippet("/Users/a t/bin/pvault")
	want := "[[ -x '/Users/a t/bin/pvault' ]] && eval \"$('/Users/a t/bin/pvault' env --zsh)\""
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRunEnv(t *testing.T) {
	m := mapping.Mapping{Secrets: []mapping.Secret{
		{Pass: "a", Env: "A", Scope: mapping.ScopeGlobal},
		{Pass: "b", Env: "B", Scope: mapping.ScopeCommand, Commands: []string{"gh"}},
		{Pass: "c", Scope: mapping.ScopeNone},
		{Pass: "d", Env: "", Scope: mapping.ScopeGlobal},
	}}
	read := readValues(map[string][]byte{"a": []byte("1"), "b": []byte("2")})

	pairs, err := RunEnv(context.Background(), m, RunRequest{Names: nil, Read: read})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(pairs, ",") != "A=1,B=2" {
		t.Fatalf("pairs: %v", pairs)
	}

	pairs, err = RunEnv(context.Background(), m, RunRequest{Names: []string{"b"}, Read: read})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(pairs, ",") != "B=2" {
		t.Fatalf("pairs: %v", pairs)
	}

	if _, err := RunEnv(context.Background(), m, RunRequest{Names: []string{"nope"}, Read: read}); err == nil {
		t.Fatal("unknown name: want error")
	}

	broken := func(_ context.Context, _ string) ([]byte, error) { return nil, errors.New("boom") }
	if _, err := RunEnv(context.Background(), m, RunRequest{Names: []string{"a"}, Read: broken}); err == nil {
		t.Fatal("read error: want error")
	}
}
