package syncer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

// Source reads a secret value by reference. opcli.Client satisfies it.
type Source interface {
	Read(ctx context.Context, ref string) ([]byte, error)
}

// Vault is the pass store. store.Store satisfies it.
type Vault interface {
	Show(ctx context.Context, name string) ([]byte, error)
	Exists(name string) bool
	Insert(ctx context.Context, name string, value []byte) error
	Remove(ctx context.Context, name string) error
	List() ([]string, error)
	Dirty(ctx context.Context) (bool, error)
	Tag(ctx context.Context, name string) error
	Head(ctx context.Context) (string, error)
	IsGitRepo() bool
}

// Action classifies one entry after comparing source and vault.
type Action string

const (
	ActionAdd       Action = "add"
	ActionUnchanged Action = "unchanged"
	ActionDiffers   Action = "differs"
	ActionFailed    Action = "failed"
	ActionOrphan    Action = "orphan"
)

// Entry is one secret's plan or outcome. Lengths only, never the value.
type Entry struct {
	Secret    mapping.Secret
	Action    Action
	Err       error
	SourceLen int
	VaultLen  int
}

// Decision answers a conflict prompt.
type Decision int

const (
	DecisionOverwrite Decision = iota
	DecisionSkip
	DecisionQuit
)

// Conflict describes one differing entry to the Decider. Reveal re-reads
// both values on request; the syncer never holds them after deciding.
type Conflict struct {
	Secret    mapping.Secret
	SourceLen int
	VaultLen  int
	Reveal    func() (source, vault []byte, err error)
}

// Decider resolves conflicts, e.g. the TUI prompt.
type Decider interface {
	Decide(ctx context.Context, conflict Conflict) (Decision, error)
}

// SyncOptions tunes one Sync run.
type SyncOptions struct {
	Only          []string
	Prune         bool
	SkipConflicts bool
	DryRun        bool
}

// Report summarizes one Sync run.
type Report struct {
	Entries   []Entry
	Added     int
	Updated   int
	Unchanged int
	Skipped   int
	Failed    int
	Orphans   int
	Pruned    int
	Tag       string
	Quit      bool
}

// ExitCode is 2 when any entry failed, else 0.
func (r Report) ExitCode() int {
	if r.Failed > 0 {
		return 2
	}
	return 0
}

// Differs counts entries left differing (skipped conflicts).
func (r Report) Differs() int {
	count := 0
	for _, entry := range r.Entries {
		if entry.Action == ActionDiffers {
			count++
		}
	}
	return count
}

// Syncer syncs one mapping from Source to Vault.
type Syncer struct {
	Source  Source
	Vault   Vault
	State   *State
	Now     func() time.Time
	Decider Decider
}

// run is one Sync in flight: its options and the report it builds.
type run struct {
	opts   SyncOptions
	report *Report
}

// Check compares source and vault without writing anything.
func (s *Syncer) Check(ctx context.Context, m mapping.Mapping, only []string) ([]Entry, error) {
	var entries []Entry
	for _, secret := range m.Secrets {
		if len(only) > 0 && !containsName(only, secret.Pass) {
			continue
		}
		if secret.Ref == "" { // local entry: never read from 1Password
			continue
		}
		entries = append(entries, s.checkEntry(ctx, secret))
	}
	return append(entries, s.orphanEntries(m)...), nil
}

func (s *Syncer) checkEntry(ctx context.Context, secret mapping.Secret) Entry {
	entry := Entry{Secret: secret}
	source, err := s.Source.Read(ctx, secret.Ref)
	if err != nil {
		entry.Action, entry.Err = ActionFailed, err
		return entry
	}
	defer clear(source)
	entry.SourceLen = len(source)
	if !s.Vault.Exists(secret.Pass) {
		entry.Action = ActionAdd
		return entry
	}
	vault, err := s.Vault.Show(ctx, secret.Pass)
	if err != nil {
		entry.Action, entry.Err = ActionFailed, err
		return entry
	}
	defer clear(vault)
	entry.VaultLen = len(vault)
	if bytes.Equal(source, vault) {
		entry.Action = ActionUnchanged
	} else {
		entry.Action = ActionDiffers
	}
	return entry
}

// orphanEntries lists state entries no longer in the mapping.
func (s *Syncer) orphanEntries(m mapping.Mapping) []Entry {
	var orphans []Entry
	for name := range s.State.Entries {
		if _, ok := m.Find(name); !ok {
			orphans = append(orphans, Entry{Secret: mapping.Secret{Pass: name}, Action: ActionOrphan})
		}
	}
	sort.Slice(orphans, func(i, j int) bool { return orphans[i].Secret.Pass < orphans[j].Secret.Pass })
	return orphans
}

// Sync applies the mapping: insert new, resolve conflicts, prune orphans.
func (s *Syncer) Sync(ctx context.Context, m mapping.Mapping, opts SyncOptions) (Report, error) {
	if err := s.checkDirty(ctx, opts.DryRun); err != nil {
		return Report{}, err
	}
	entries, err := s.Check(ctx, m, opts.Only)
	if err != nil {
		return Report{}, err
	}
	report := Report{Entries: entries}
	run := run{opts: opts, report: &report}
	for i := range report.Entries {
		if err := s.applyEntry(ctx, run, i); err != nil {
			return Report{}, err
		}
		if report.Quit {
			break
		}
	}
	if err := s.finish(ctx, &report, opts.DryRun); err != nil {
		return Report{}, err
	}
	return report, nil
}

func (s *Syncer) checkDirty(ctx context.Context, dryRun bool) error {
	if dryRun || !s.Vault.IsGitRepo() {
		return nil
	}
	dirty, err := s.Vault.Dirty(ctx)
	if err != nil {
		return err
	}
	if dirty {
		return errors.New("pass store has uncommitted changes; commit or discard them (pass git status)")
	}
	return nil
}

// applyEntry performs one entry's action and updates the report counters.
func (s *Syncer) applyEntry(ctx context.Context, run run, index int) error {
	entry := &run.report.Entries[index]
	switch entry.Action {
	case ActionAdd:
		return s.applyInsert(ctx, run, entry)
	case ActionUnchanged:
		run.report.Unchanged++
		s.markSynced(entry, false)
	case ActionDiffers:
		return s.applyConflict(ctx, run, entry)
	case ActionFailed:
		run.report.Failed++
	case ActionOrphan:
		run.report.Orphans++
		return s.applyOrphan(ctx, run, entry)
	}
	return nil
}

// applyInsert inserts the source value, unless DryRun. A new entry counts
// Added, an overwrite counts Updated; both end up as ActionAdd so the
// report no longer shows them as differing.
func (s *Syncer) applyInsert(ctx context.Context, run run, entry *Entry) error {
	overwriting := entry.Action == ActionDiffers
	if !run.opts.DryRun {
		value, err := s.Source.Read(ctx, entry.Secret.Ref)
		if err != nil {
			entry.Action, entry.Err = ActionFailed, err
			run.report.Failed++
			if overwriting {
				run.report.Updated++
			} else {
				run.report.Added++
			}
			return nil
		}
		defer clear(value)
		if err := s.Vault.Insert(ctx, entry.Secret.Pass, value); err != nil {
			return err
		}
	}
	entry.Action = ActionAdd
	if overwriting {
		run.report.Updated++
	} else {
		run.report.Added++
	}
	s.markSynced(entry, true)
	return nil
}

func (s *Syncer) applyConflict(ctx context.Context, run run, entry *Entry) error {
	if run.opts.SkipConflicts {
		run.report.Skipped++
		return nil
	}
	if s.Decider == nil {
		return fmt.Errorf("entry %s differs and there is no decider", entry.Secret.Pass)
	}
	decision, err := s.Decider.Decide(ctx, s.conflictOf(ctx, entry))
	if err != nil {
		return err
	}
	switch decision {
	case DecisionOverwrite:
		return s.applyInsert(ctx, run, entry)
	case DecisionSkip:
		run.report.Skipped++
	case DecisionQuit:
		run.report.Quit = true
	}
	return nil
}

// conflictOf builds the Conflict for entry; Reveal re-reads both values.
func (s *Syncer) conflictOf(ctx context.Context, entry *Entry) Conflict {
	return Conflict{
		Secret:    entry.Secret,
		SourceLen: entry.SourceLen,
		VaultLen:  entry.VaultLen,
		Reveal: func() ([]byte, []byte, error) {
			source, err := s.Source.Read(ctx, entry.Secret.Ref)
			if err != nil {
				return nil, nil, err
			}
			vault, err := s.Vault.Show(ctx, entry.Secret.Pass)
			if err != nil {
				clear(source)
				return nil, nil, err
			}
			return source, vault, nil
		},
	}
}

// applyOrphan removes a pruned orphan and drops it from state.
func (s *Syncer) applyOrphan(ctx context.Context, run run, entry *Entry) error {
	if !run.opts.Prune {
		return nil
	}
	if !run.opts.DryRun {
		if err := s.Vault.Remove(ctx, entry.Secret.Pass); err != nil {
			return err
		}
		delete(s.State.Entries, entry.Secret.Pass)
	}
	run.report.Pruned++
	return nil
}

// markSynced records entry's pass path in state; tag is filled in finish.
// refresh updates SyncedAt for adds and overwrites; an unchanged entry
// keeps its first sync time.
func (s *Syncer) markSynced(entry *Entry, refresh bool) {
	state, ok := s.State.Entries[entry.Secret.Pass]
	if ok && !refresh {
		return
	}
	state.SyncedAt = s.Now()
	s.State.Entries[entry.Secret.Pass] = state
}

// finish tags the store when anything changed and stamps state entries.
func (s *Syncer) finish(ctx context.Context, report *Report, dryRun bool) error {
	changed := report.Added+report.Updated+report.Pruned > 0
	if !changed {
		return nil
	}
	if dryRun || !s.Vault.IsGitRepo() {
		return nil
	}
	tag, err := s.tagUnique(ctx)
	if err != nil {
		return err
	}
	report.Tag, s.State.LastTag = tag, tag
	for _, entry := range report.Entries {
		if entry.Action != ActionAdd && entry.Action != ActionUnchanged {
			continue
		}
		if state, ok := s.State.Entries[entry.Secret.Pass]; ok {
			state.Tag = tag
			s.State.Entries[entry.Secret.Pass] = state
		}
	}
	return nil
}

// tagUnique tags the store with a timestamp name; two runs inside one second
// collide, so it retries with a numeric suffix until git accepts the tag.
func (s *Syncer) tagUnique(ctx context.Context) (string, error) {
	base := "sync-" + s.Now().UTC().Format("20060102T150405")
	tag := base
	for suffix := 1; ; suffix++ {
		err := s.Vault.Tag(ctx, tag)
		if err == nil {
			return tag, nil
		}
		if suffix > 9 {
			return "", err
		}
		tag = fmt.Sprintf("%s-%d", base, suffix)
	}
}

func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}
