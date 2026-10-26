// Package mapping loads and validates ~/.config/pvault/mapping.toml, the
// per-machine list of 1Password references to copy into pass.
package mapping

// CurrentVersion is the newest mapping.toml version this binary reads.
const CurrentVersion = 1

// Scope controls how a secret reaches the shell.
type Scope string

const (
	// ScopeGlobal exports the variable in every shell (the default).
	ScopeGlobal Scope = "global"
	// ScopeCommand sets the variable only for the listed commands.
	ScopeCommand Scope = "command"
	// ScopeNone syncs the secret but never puts it in the environment.
	ScopeNone Scope = "none"
)

// Secret is one [[secret]] table.
type Secret struct {
	Pass     string   `toml:"pass"`
	Ref      string   `toml:"ref"`
	Env      string   `toml:"env"`
	Scope    Scope    `toml:"scope"`
	Commands []string `toml:"commands"`
}

// Mapping is the whole file.
type Mapping struct {
	Version   int      `toml:"version"`
	OpAccount string   `toml:"op_account"`
	Secrets   []Secret `toml:"secret"`
}

// Find returns the secret with the given pass path.
func (m Mapping) Find(pass string) (Secret, bool) {
	for _, secret := range m.Secrets {
		if secret.Pass == pass {
			return secret, true
		}
	}
	return Secret{}, false
}

// Example is the commented file that `pvault mapping init` and setup write.
const Example = `# pvault mapping file. One [[secret]] table per 1Password field to copy into pass.
# Docs: docs/mapping.md in the pvault repo. Validate with: pvault mapping validate
version = 1
# op_account = "my.1password.com"   # only if you have more than one 1Password account

# [[secret]]
# pass = "pagerduty-api-key"                  # path in the pass store, without .gpg
# ref  = "op://Private/PagerDuty/credential"   # 1Password: field menu > Copy Secret Reference
# env  = "PAGERDUTY_API_KEY"                   # optional: environment variable name
# scope = "global"                             # global (default) | command | none

# [[secret]]
# pass     = "work/github-token"
# ref      = "op://Employee/GitHub PAT/token"
# env      = "GITHUB_TOKEN"
# scope    = "command"
# commands = ["gh"]                            # set GITHUB_TOKEN only when gh runs
`
