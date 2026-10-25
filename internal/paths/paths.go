// Package paths resolves every file location pvault reads or writes.
package paths

import (
	"os"
	"path/filepath"
)

// Paths holds the base directories. Build it with FromEnv.
type Paths struct {
	Home      string
	ConfigDir string // $XDG_CONFIG_HOME/pvault
	StateDir  string // $XDG_STATE_HOME/pvault
	BinDir    string // ~/.local/bin
	StoreDir  string // $PASSWORD_STORE_DIR or ~/.password-store
	GnupgHome string // $GNUPGHOME or ~/.gnupg
}

// FromEnv reads HOME, XDG_CONFIG_HOME, XDG_STATE_HOME, PASSWORD_STORE_DIR
// and GNUPGHOME, with the documented defaults for each.
func FromEnv() Paths {
	home := os.Getenv("HOME")
	return Paths{
		Home:      home,
		ConfigDir: filepath.Join(envOr("XDG_CONFIG_HOME", filepath.Join(home, ".config")), "pvault"),
		StateDir:  filepath.Join(envOr("XDG_STATE_HOME", filepath.Join(home, ".local", "state")), "pvault"),
		BinDir:    filepath.Join(home, ".local", "bin"),
		StoreDir:  envOr("PASSWORD_STORE_DIR", filepath.Join(home, ".password-store")),
		GnupgHome: envOr("GNUPGHOME", filepath.Join(home, ".gnupg")),
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// Mapping is the user's mapping.toml.
func (p Paths) Mapping() string { return filepath.Join(p.ConfigDir, "mapping.toml") }

// Manifest records what setup installed.
func (p Paths) Manifest() string { return filepath.Join(p.StateDir, "manifest.json") }

// State records the last sync of each managed entry.
func (p Paths) State() string { return filepath.Join(p.StateDir, "state.json") }

// Binary is the installed pvault executable.
func (p Paths) Binary() string { return filepath.Join(p.BinDir, "pvault") }

// Zshrc is the shell file that gets the managed block.
func (p Paths) Zshrc() string { return filepath.Join(p.Home, ".zshrc") }

// GpgAgentConf is the gpg-agent config that gets the pinentry block.
func (p Paths) GpgAgentConf() string { return filepath.Join(p.GnupgHome, "gpg-agent.conf") }
