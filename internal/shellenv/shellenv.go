// Package shellenv renders the zsh script that `pvault env --zsh` prints and
// the KEY=VALUE pairs that `pvault run` sets.
package shellenv

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

// ReadSecret returns the exact bytes of one pass entry.
type ReadSecret func(ctx context.Context, passName string) ([]byte, error)

// Warning names one entry RenderScript skipped and why.
type Warning struct {
	Pass   string
	Reason string
}

// Quote wraps value in POSIX single quotes and escapes embedded quotes.
// Safe for any bytes except NUL, which cannot appear in an environment
// variable.
func Quote(value []byte) string {
	return "'" + strings.ReplaceAll(string(value), "'", `'\''`) + "'"
}

// RenderScript returns the zsh script: exports for global secrets, then one wrapper
// function per command. A read error or a NUL byte skips the entry with a
// warning; RenderScript never fails. Command-scope secrets are not read here.
func RenderScript(ctx context.Context, m mapping.Mapping, read ReadSecret) (string, []Warning) {
	var lines []string
	var warnings []Warning
	for _, secret := range m.Secrets {
		if secret.Scope != mapping.ScopeGlobal || secret.Env == "" {
			continue
		}
		value, err := read(ctx, secret.Pass)
		if err != nil {
			warnings = append(warnings, Warning{secret.Pass, err.Error()})
			continue
		}
		if strings.IndexByte(string(value), 0) >= 0 {
			warnings = append(warnings, Warning{secret.Pass, "value contains NUL byte"})
			continue
		}
		lines = append(lines, "export "+secret.Env+"="+Quote(value))
	}
	for _, command := range commandOrder(m) {
		lines = append(lines, commandFunction(command, m))
	}
	if len(lines) == 0 {
		return "", warnings
	}
	return strings.Join(lines, "\n") + "\n", warnings
}

// Snippet returns the zshrc body line that loads the environment when the
// binary at binaryPath exists.
func Snippet(binaryPath string) string {
	quoted := Quote([]byte(binaryPath))
	return "[[ -x " + quoted + " ]] && eval \"$(" + quoted + " env --zsh)\""
}

// RunRequest says which entries `pvault run` needs and how to read them.
// Empty Names means every entry that has an env variable.
type RunRequest struct {
	Names []string
	Read  ReadSecret
}

// RunEnv returns KEY=VALUE pairs for the named pass entries, or for every
// entry with Env set when Names is empty. Unlike RenderScript, a read error or an
// unknown name is an error.
func RunEnv(ctx context.Context, m mapping.Mapping, request RunRequest) ([]string, error) {
	secrets, err := selectSecrets(m, request.Names)
	if err != nil {
		return nil, err
	}
	var pairs []string
	for _, secret := range secrets {
		value, err := request.Read(ctx, secret.Pass)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", secret.Pass, err)
		}
		pairs = append(pairs, secret.Env+"="+string(value))
	}
	return pairs, nil
}

func selectSecrets(m mapping.Mapping, names []string) ([]mapping.Secret, error) {
	if len(names) == 0 {
		var secrets []mapping.Secret
		for _, secret := range m.Secrets {
			if secret.Env != "" {
				secrets = append(secrets, secret)
			}
		}
		return secrets, nil
	}
	secrets := make([]mapping.Secret, 0, len(names))
	for _, name := range names {
		secret, ok := m.Find(name)
		if !ok {
			return nil, fmt.Errorf("unknown pass entry: %s", name)
		}
		if secret.Env != "" {
			secrets = append(secrets, secret)
		}
	}
	return secrets, nil
}

// commandOrder returns the command names that have secrets, sorted.
func commandOrder(m mapping.Mapping) []string {
	seen := map[string]bool{}
	var commands []string
	for _, secret := range m.Secrets {
		if secret.Scope != mapping.ScopeCommand || secret.Env == "" {
			continue
		}
		for _, command := range secret.Commands {
			if !seen[command] {
				seen[command] = true
				commands = append(commands, command)
			}
		}
	}
	sort.Strings(commands)
	return commands
}

// commandFunction renders one wrapper function that sets every command-scope
// variable of command for that one invocation only.
func commandFunction(command string, m mapping.Mapping) string {
	var assignments []string
	for _, secret := range m.Secrets {
		if secret.Scope != mapping.ScopeCommand || secret.Env == "" {
			continue
		}
		for _, c := range secret.Commands {
			if c == command {
				assignments = append(assignments, secret.Env+"=\"$(command pass show "+Quote([]byte(secret.Pass))+")\"")
			}
		}
	}
	return command + "() { " + strings.Join(assignments, " ") + " command " + command + " \"$@\"; }"
}
