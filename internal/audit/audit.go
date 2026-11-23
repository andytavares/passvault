// Package audit appends who used which pass entry, how and when, to a
// hash-chained JSON Lines log. It never records a secret value, its length
// or a hash of it: events hold entry names, actions and process details only.
package audit

import (
	"os"
	"os/user"
	"time"
)

// Action is what a pvault command did with an entry.
type Action string

// The actions an Event can record.
const (
	ActionUse      Action = "use"
	ActionExport   Action = "export"
	ActionCompare  Action = "compare"
	ActionView     Action = "view"
	ActionCopy     Action = "copy"
	ActionWrite    Action = "write"
	ActionDelete   Action = "delete"
	ActionRollback Action = "rollback"
	ActionAuditOn  Action = "audit-on"
	ActionAuditOff Action = "audit-off"
)

// Process is one process in the chain that started pvault.
type Process struct {
	PID  int      `json:"pid"`
	Comm string   `json:"comm"`
	Argv []string `json:"argv"`
}

// Event is one line of the audit log. Prev is the hex SHA-256 of the line
// before it, or empty for the first line.
type Event struct {
	Time    time.Time `json:"time"`
	Entry   string    `json:"entry"`
	Action  Action    `json:"action"`
	Via     string    `json:"via"`
	User    string    `json:"user"`
	UID     int       `json:"uid"`
	Host    string    `json:"host"`
	PID     int       `json:"pid"`
	Callers []Process `json:"callers"`
	Detail  string    `json:"detail,omitempty"`
	Prev    string    `json:"prev"`
}

// Recorder writes events for audited entries to the log at Path.
type Recorder struct {
	Path    string                  // audit.jsonl
	Via     string                  // pvault subcommand, e.g. "get", "tui"
	Now     func() time.Time        // event time source
	Audited func(entry string) bool // whether entry has the audit flag
	Callers func() []Process        // nil means CallerChain(os.Getppid())
}

// Record appends one event when entry is audited. Audit-on and audit-off
// events are always written. A nil Recorder records nothing.
func (r *Recorder) Record(entry string, action Action, detail string) error {
	if r == nil {
		return nil
	}
	if action != ActionAuditOn && action != ActionAuditOff && !r.Audited(entry) {
		return nil
	}
	event, err := r.newEvent(entry, action, detail)
	if err != nil {
		return err
	}
	return appendEvent(r.Path, event)
}

func (r *Recorder) newEvent(entry string, action Action, detail string) (Event, error) {
	current, err := user.Current()
	if err != nil {
		return Event{}, err
	}
	host, err := os.Hostname()
	if err != nil {
		return Event{}, err
	}
	return Event{
		Time: r.Now().UTC(), Entry: entry, Action: action, Via: r.Via,
		User: current.Username, UID: os.Getuid(), Host: host, PID: os.Getpid(),
		Callers: r.callers(), Detail: detail,
	}, nil
}

func (r *Recorder) callers() []Process {
	if r.Callers == nil {
		return CallerChain(os.Getppid())
	}
	return r.Callers()
}
