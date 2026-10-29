package mapping

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validSecret() Secret {
	return Secret{Pass: "pagerduty-api-key", Ref: "op://Private/PagerDuty/credential"}
}

func with(secrets ...Secret) Mapping {
	return Mapping{Version: CurrentVersion, Secrets: secrets}
}

type validateCase struct {
	name    string
	mapping Mapping
	want    string // empty means valid
}

var validateCases = []validateCase{
	{"empty mapping is valid", with(), ""},
	{"minimal secret is valid", with(valid), ""},
	{"command scope is valid", with(scoped), ""},
	{"version missing", Mapping{Secrets: []Secret{valid}}, "version is required"},
	{"version too new", Mapping{Version: CurrentVersion + 1, Secrets: []Secret{valid}}, "newer than this pvault supports"},
	{"pass empty", with(Secret{Ref: "op://V/I/f"}), "pass is empty"},
	{"pass leading slash", with(Secret{Pass: "/a", Ref: "op://V/I/f"}), "leading"},
	{"pass dot-dot segment", with(Secret{Pass: "a/../b", Ref: "op://V/I/f"}), ".."},
	{"pass gpg suffix", with(Secret{Pass: "a.gpg", Ref: "op://V/I/f"}), ".gpg"},
	{"duplicate pass", with(valid, valid), "duplicate pass"},
	{"ref missing op prefix", with(Secret{Pass: "a", Ref: "Private/I/f"}), "op://"},
	{"env lowercase", with(Secret{Pass: "a", Ref: "op://V/I/f", Env: "github_token"}), "env"},
	{"env leading digit", with(Secret{Pass: "a", Ref: "op://V/I/f", Env: "1TOKEN"}), "env"},
	{"duplicate env", with(
		Secret{Pass: "a", Ref: "op://V/I/f", Env: "TOKEN"},
		Secret{Pass: "b", Ref: "op://V/J/f", Env: "TOKEN"},
	), "duplicate env"},
	{"scope unknown", with(Secret{Pass: "a", Ref: "op://V/I/f", Scope: "sometimes"}), "scope"},
	{"command scope without env", with(Secret{Pass: "a", Ref: "op://V/I/f", Scope: ScopeCommand, Commands: []string{"gh"}}), "env"},
	{"command scope without commands", with(Secret{Pass: "a", Ref: "op://V/I/f", Env: "TOKEN", Scope: ScopeCommand}), "commands"},
	{"command name has shell metachar", with(Secret{Pass: "a", Ref: "op://V/I/f", Env: "TOKEN", Scope: ScopeCommand, Commands: []string{"gh;rm"}}), "commands"},
	{"commands without command scope", with(Secret{Pass: "a", Ref: "op://V/I/f", Commands: []string{"gh"}}), "commands"},
	{"local entry with env is valid", with(Secret{Pass: "a", Env: "TOKEN"}), ""},
	{"local entry without env", with(Secret{Pass: "a"}), "needs env"},
}

var (
	valid  = validSecret()
	scoped = Secret{
		Pass:     "work/github-token",
		Ref:      "op://Employee/GitHub PAT/token",
		Env:      "GITHUB_TOKEN",
		Scope:    ScopeCommand,
		Commands: []string{"gh"},
	}
)

func TestValidate(t *testing.T) {
	for _, tt := range validateCases {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mapping.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestValidateJoinsAllProblems(t *testing.T) {
	err := with(
		Secret{Ref: "bad"},
		Secret{Ref: "bad"},
	).Validate()
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"secret[0]", "secret[1]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q misses %q", err, want)
		}
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok || len(joined.Unwrap()) < 4 {
		t.Errorf("want >=4 joined problems, got %v", err)
	}
}

func TestParseRejectsUnknownKey(t *testing.T) {
	_, err := Parse([]byte("version = 1\n\n[[secret]]\npass = \"a\"\nref = \"op://V/I/f\"\npas = \"x\"\n"))
	if err == nil || !strings.Contains(err.Error(), "pas") {
		t.Fatalf("want unknown-key error naming pas, got %v", err)
	}
}

func TestParseDefaultsScopeToGlobal(t *testing.T) {
	m, err := Parse([]byte("version = 1\n\n[[secret]]\npass = \"a\"\nref = \"op://V/I/f\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Secrets[0].Scope != ScopeGlobal {
		t.Fatalf("scope = %q, want %q", m.Secrets[0].Scope, ScopeGlobal)
	}
}

func TestExampleParsesAndValidates(t *testing.T) {
	m, err := Parse([]byte(Example))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Secrets) != 0 {
		t.Fatalf("Example has %d secrets, want 0", len(m.Secrets))
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "mapping.toml"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want os.ErrNotExist, got %v", err)
	}
}

func TestLoadErrorNamesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.toml")
	if err := os.WriteFile(path, []byte("not toml = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("want error naming %s, got %v", path, err)
	}
}

func TestWriteExampleCreatesParentsAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pvault", "mapping.toml")
	created, err := WriteExample(path)
	if err != nil || !created {
		t.Fatalf("created %v err %v", created, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o, want 600", info.Mode().Perm())
	}
	data, _ := os.ReadFile(path)
	if string(data) != Example {
		t.Fatal("content differs from Example")
	}
}

func TestWriteExampleNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.toml")
	mine := "# mine\nversion = 1\n"
	if err := os.WriteFile(path, []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := WriteExample(path)
	if err != nil || created {
		t.Fatalf("created %v err %v", created, err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != mine {
		t.Fatalf("overwrote existing file: %q", data)
	}
}

func TestAppendSecretKeepsExistingBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.toml")
	existing := "# my comment\nversion = 1\n\n[[secret]]\npass = \"a\"\nref = \"op://V/I/f\"\n"
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	added := Secret{Pass: "b", Ref: "op://V/J/f", Env: "TOKEN_B", Scope: ScopeGlobal}
	if err := AppendSecret(path, added); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), existing) {
		t.Fatalf("existing bytes changed:\n%q", data)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	found, ok := m.Find("b")
	if !ok || found.Env != "TOKEN_B" || found.Scope != ScopeGlobal {
		t.Fatalf("loaded %+v ok %v", found, ok)
	}
	if strings.Contains(string(data), "scope") {
		t.Errorf("global scope must be omitted:\n%s", data)
	}
}

func TestAppendSecretRejectsDuplicatePass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.toml")
	existing := "version = 1\n\n[[secret]]\npass = \"a\"\nref = \"op://V/I/f\"\n"
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	err := AppendSecret(path, Secret{Pass: "a", Ref: "op://V/J/f"})
	if err == nil || !strings.Contains(err.Error(), "duplicate pass") {
		t.Fatalf("want duplicate pass error, got %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != existing {
		t.Fatalf("file changed after rejection:\n%q", data)
	}
}

func TestRemoveSecretKeepsOtherBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.toml")
	head := "# my comment\nversion = 1\n\n[[secret]]\npass = \"a\"\nref = \"op://V/I/f\"\n\n"
	tail := "[[secret]]\npass = \"c\"\nref = \"op://V/K/f\"\n"
	removed := "[[secret]]\npass = \"b\"\nref = \"op://V/J/f\"\n\n"
	if err := os.WriteFile(path, []byte(head+removed+tail), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSecret(path, "b"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != head+tail {
		t.Fatalf("got:\n%q", data)
	}
}

func TestRemoveSecretMissingPassFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.toml")
	existing := "version = 1\n\n[[secret]]\npass = \"a\"\nref = \"op://V/I/f\"\n"
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSecret(path, "b"); err == nil {
		t.Fatal("want error for a pass that is not mapped")
	}
}

func TestParseEnabledDefaultsOn(t *testing.T) {
	m, err := Parse([]byte("version = 1\n[[secret]]\npass = \"a\"\nenv = \"A\"\n[[secret]]\npass = \"b\"\nenv = \"B\"\nenabled = false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !m.Secrets[0].IsEnabled() || m.Secrets[1].IsEnabled() {
		t.Fatalf("enabled = %v, %v; want true, false", m.Secrets[0].IsEnabled(), m.Secrets[1].IsEnabled())
	}
}

func TestAppendLocalDisabledSecretRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.toml")
	if err := os.WriteFile(path, []byte("version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	off := false
	if err := AppendSecret(path, Secret{Pass: "notes/wifi", Env: "WIFI_PASSWORD", Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "ref") || !strings.Contains(string(data), "enabled = false") {
		t.Fatalf("want no ref line and enabled = false:\n%s", data)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Find("notes/wifi"); got.IsEnabled() || got.Ref != "" {
		t.Fatalf("loaded %+v", got)
	}
}

func TestReplaceSecretKeepsOtherBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.toml")
	head := "# my comment\nversion = 1\n\n[[secret]]\npass = \"a\"\nref = \"op://V/I/f\"\n\n"
	tail := "[[secret]]\npass = \"c\"\nref = \"op://V/K/f\"\n"
	old := "[[secret]]\npass = \"b\"\nref = \"op://V/J/f\"\n\n"
	if err := os.WriteFile(path, []byte(head+old+tail), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceSecret(path, Secret{Pass: "b", Ref: "op://V/J/f", Env: "TOKEN_B", Scope: ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := head + "[[secret]]\npass = \"b\"\nref = \"op://V/J/f\"\nenv = \"TOKEN_B\"\n\n" + tail
	if string(data) != want {
		t.Fatalf("got:\n%q\nwant:\n%q", data, want)
	}
}

func TestReplaceSecretRejectsInvalidResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.toml")
	existing := "version = 1\n\n[[secret]]\npass = \"a\"\nref = \"op://V/I/f\"\nenv = \"A\"\n\n[[secret]]\npass = \"b\"\nref = \"op://V/J/f\"\n"
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	err := ReplaceSecret(path, Secret{Pass: "b", Ref: "op://V/J/f", Env: "A"})
	if err == nil || !strings.Contains(err.Error(), "duplicate env") {
		t.Fatalf("want duplicate env error, got %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != existing {
		t.Fatalf("file changed on a rejected replace:\n%s", data)
	}
}
