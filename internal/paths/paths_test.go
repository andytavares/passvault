package paths

import "testing"

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("HOME", "/h")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("PASSWORD_STORE_DIR", "")
	t.Setenv("GNUPGHOME", "")
	p := FromEnv()
	want := map[string]string{
		p.Mapping():      "/h/.config/pvault/mapping.toml",
		p.Manifest():     "/h/.local/state/pvault/manifest.json",
		p.Binary():       "/h/.local/bin/pvault",
		p.StoreDir:       "/h/.password-store",
		p.GpgAgentConf(): "/h/.gnupg/gpg-agent.conf",
	}
	for got, expected := range want {
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("HOME", "/h")
	t.Setenv("XDG_CONFIG_HOME", "/c")
	t.Setenv("PASSWORD_STORE_DIR", "/s")
	t.Setenv("GNUPGHOME", "/g")
	p := FromEnv()
	if p.Mapping() != "/c/pvault/mapping.toml" || p.StoreDir != "/s" || p.GpgAgentConf() != "/g/gpg-agent.conf" {
		t.Fatalf("overrides ignored: %+v", p)
	}
}
