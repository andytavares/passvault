package cli

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
	"github.com/atavares-klaviyo/pass-vault/internal/tui"
)

// registerTUICommand makes plain `pvault` open the TUI. Without a terminal
// on both ends it prints help, so pipes and scripts never block on it.
func registerTUICommand(root *cobra.Command, a *app) {
	root.Args = cobra.NoArgs
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		if !isTerminal(a.in) || !isTerminal(a.out) {
			return cmd.Help()
		}
		return tui.Run(a.tuiDeps())
	}
}

func (a *app) tuiDeps() tui.Deps {
	client := a.opClient(mapping.Mapping{})
	if m, err := a.loadMapping(); err == nil {
		client = a.opClient(m)
	}
	return tui.Deps{
		Mapping:     a.loadMapping,
		MappingPath: a.paths.Mapping(),
		Store:       a.store(),
		Source:      client,
		Items:       client,
		State:       func() (syncer.State, error) { return syncer.LoadState(a.paths.State()) },
		SaveState:   func(state syncer.State) error { return syncer.SaveState(a.paths.State(), state) },
		Now:         a.now,
		Editor:      editorCommand,
	}
}

// editorCommand returns $EDITOR (vi when unset) opening path. EDITOR is a
// single program name, as `git` and `crontab` treat it by default.
func editorCommand(path string) *exec.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	return exec.Command(editor, path)
}

func isTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
