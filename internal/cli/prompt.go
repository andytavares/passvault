package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

// cliDecider resolves sync conflicts with a terminal prompt. It prints
// values only when the user picks reveal.
type cliDecider struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

func (d *cliDecider) Decide(_ context.Context, conflict syncer.Conflict) (syncer.Decision, error) {
	fmt.Fprintf(d.out, "  ~ %s  differs  1Password %d chars · pass %d chars\n",
		conflict.Secret.Pass, conflict.SourceLen, conflict.VaultLen)
	reader := bufio.NewReader(d.in)
	for {
		fmt.Fprint(d.out, "Overwrite pass entry? [o]verwrite [s]kip [v]reveal [q]uit: ")
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(d.out)
			return syncer.DecisionQuit, nil
		}
		switch strings.TrimSpace(line) {
		case "o":
			return syncer.DecisionOverwrite, nil
		case "s":
			return syncer.DecisionSkip, nil
		case "q":
			return syncer.DecisionQuit, nil
		case "v":
			if err := d.reveal(conflict); err != nil {
				fmt.Fprintf(d.errOut, "pvault: reveal: %v\n", err)
			}
		default:
			fmt.Fprintln(d.out, "Please answer o, s, v or q.")
		}
	}
}

// reveal prints both values once, on explicit request only.
func (d *cliDecider) reveal(conflict syncer.Conflict) error {
	source, vault, err := conflict.Reveal()
	if err != nil {
		return err
	}
	defer clear(source)
	defer clear(vault)
	fmt.Fprintf(d.out, "  1Password: %s\n  pass:      %s\n", source, vault)
	return nil
}
