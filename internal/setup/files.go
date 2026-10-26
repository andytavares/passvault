package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/atavares-klaviyo/pass-vault/internal/block"
	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

func (in *Installer) mappingStep() Step {
	return Step{
		Name: "mapping.toml",
		Check: func(context.Context) (bool, string, error) {
			if _, err := os.Stat(in.Paths.Mapping()); os.IsNotExist(err) {
				return false, "", nil
			}
			return true, "mapping.toml present", nil
		},
		Apply: func(context.Context) (string, error) {
			if err := writeMappingExample(in.Paths.Mapping()); err != nil {
				return "", err
			}
			return "created from example", nil
		},
	}
}

// writeMappingExample creates the mapping file 0600 under a 0700 parent.
func writeMappingExample(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(mapping.Example), 0o600)
}

func (in *Installer) zshrcStep() Step {
	return Step{
		Name:  "zshrc block",
		Check: func(ctx context.Context) (bool, string, error) { return in.zshrcDone(ctx) },
		Apply: func(ctx context.Context) (string, error) { return in.writeZshrc(ctx) },
	}
}

func (in *Installer) zshrcDone(context.Context) (bool, string, error) {
	content, err := os.ReadFile(in.Paths.Zshrc())
	if err != nil && !os.IsNotExist(err) {
		return false, "", err
	}
	if !block.Has(string(content)) {
		return false, "", nil
	}
	in.warnPassOutsideBlock(string(content))
	return true, "managed block present", nil
}

func (in *Installer) writeZshrc(context.Context) (string, error) {
	binary := in.Paths.Binary()
	snippet := "[[ -x '" + binary + "' ]] && eval \"$('" + binary + "' env --zsh)\""
	backup, err := block.UpsertFile(in.Paths.Zshrc(), snippet, in.now())
	if err != nil {
		return "", err
	}
	in.recordBlock(in.Paths.Zshrc())
	if backup != "" {
		in.Manifest.Backups = append(in.Manifest.Backups, backup)
	}
	content, err := os.ReadFile(in.Paths.Zshrc())
	if err != nil {
		return "", err
	}
	in.warnPassOutsideBlock(string(content))
	return "zshrc block written", nil
}

// warnPassOutsideBlock reports each zshrc line outside the block that
// invokes pass directly; those bypass the mapping file.
func (in *Installer) warnPassOutsideBlock(content string) {
	inside := false
	for number, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case block.Begin:
			inside = true
		case block.End:
			inside = false
		}
		if inside || !runsPass(trimmed) {
			continue
		}
		in.Out.Report(Warning, "zshrc block", fmt.Sprintf(
			"%s:%d runs pass outside the managed block; add it to mapping.toml",
			in.Paths.Zshrc(), number+1))
	}
}

func runsPass(line string) bool {
	return strings.Contains(line, "pass ") ||
		strings.Contains(line, "pass show") ||
		strings.Contains(line, "$(pass")
}
