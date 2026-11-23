package audit

import (
	"path/filepath"
	"strings"
)

// What is the WHAT column of pvault audit. Write-like actions show their
// Detail; reads show the caller chain, after the run child command if any.
func (e Event) What() string {
	switch e.Action {
	case ActionWrite, ActionDelete, ActionRollback, ActionAuditOn, ActionAuditOff:
		if e.Detail != "" {
			return e.Detail
		}
		return e.chain()
	}
	if e.Detail != "" {
		return e.Detail + " ← " + e.chain()
	}
	return e.chain()
}

// chain renders the callers nearest first. A subshell fork repeats its
// parent, so consecutive identical renderings collapse into one.
func (e Event) chain() string {
	var rendered []string
	for _, caller := range e.Callers {
		text := caller.render()
		if len(rendered) == 0 || rendered[len(rendered)-1] != text {
			rendered = append(rendered, text)
		}
	}
	return strings.Join(rendered, " ← ")
}

func (p Process) render() string {
	if len(p.Argv) == 0 {
		return p.Comm
	}
	return strings.Join(append([]string{filepath.Base(p.Argv[0])}, p.Argv[1:]...), " ")
}
