package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

// conflictState is the open conflict modal. The decider goroutine blocks on
// answer until the user picks a key. box is the shared copy the test hook
// and the sync goroutine both see.
type conflictState struct {
	conflict  syncer.Conflict
	answer    chan syncer.Decision
	reveal    chan revealBothMsg
	source    string
	vault     string
	revealEnd time.Time
}

// conflictBox is the shared modal cell: the sync goroutine's hook writes it
// while Update and View read it, so all access goes through its mutex.
type conflictBox struct {
	mu    sync.Mutex
	state *conflictState
}

// get returns the open conflict, if any.
func (b *conflictBox) get() *conflictState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// set opens or closes the conflict modal.
func (b *conflictBox) set(state *conflictState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = state
}

// conflictMsg arrives from the decider goroutine; index orders conflicts.
type conflictMsg struct {
	conflict syncer.Conflict
	answer   chan syncer.Decision
	reveal   chan revealBothMsg
}

// revealBothMsg carries both values after a [v] reveal request.
type revealBothMsg struct {
	source []byte
	vault  []byte
	err    error
}

// tuiDecider implements syncer.Decider by messaging the program and blocking
// on a channel until the user answers. deliver is program.Send in the live
// program, or a test hook that applies the message directly.
type tuiDecider struct {
	deliver func(tea.Msg)
}

// Decide implements syncer.Decider: it sends the conflict to the program and
// waits for its decision.
func (d tuiDecider) Decide(_ context.Context, conflict syncer.Conflict) (syncer.Decision, error) {
	answer := make(chan syncer.Decision, 1)
	reveal := make(chan revealBothMsg, 1)
	if d.deliver == nil {
		return syncer.DecisionSkip, nil
	}
	d.deliver(conflictMsg{conflict: conflict, answer: answer, reveal: reveal})
	return <-answer, nil
}

// conflictDeliverer picks where conflictMsg goes: the live program, or the
// test hook.
func (m *Model) conflictDeliverer() func(tea.Msg) {
	if m.conflictHook != nil {
		return m.conflictHook
	}
	if m.program != nil {
		return m.program.Send
	}
	return nil
}

// syncCmd runs the sync with the TUI decider; conflicts arrive as messages.
func syncCmd(m *Model) tea.Cmd {
	deps := m.deps
	state := m.state
	deliver := m.conflictDeliverer()
	decider := tuiDecider{deliver: deliver}
	return func() tea.Msg {
		mapping, err := deps.Mapping()
		if err != nil {
			return errMsg{err}
		}
		s := syncer.Syncer{Source: deps.Source, Vault: deps.Store, State: &state, Now: deps.Now, Decider: decider}
		report, err := s.Sync(context.Background(), mapping, syncer.SyncOptions{})
		if err != nil {
			return errMsg{err}
		}
		if deps.SaveState != nil {
			if err := deps.SaveState(state); err != nil {
				return errMsg{err}
			}
		}
		return syncDoneMsg{added: report.Added + report.Updated, unchanged: report.Unchanged, skipped: report.Skipped}
	}
}

// openConflict opens the modal and wires the reveal channel.
func (m Model) openConflict(msg conflictMsg) (tea.Model, tea.Cmd) {
	m.conflict.set(&conflictState{conflict: msg.conflict, answer: msg.answer, reveal: msg.reveal})
	return m, awaitReveal(msg.reveal)
}

// awaitReveal waits for the decider goroutine to answer a reveal request.
func awaitReveal(reveal chan revealBothMsg) tea.Cmd {
	return func() tea.Msg { return <-reveal }
}

// applyRevealBoth shows both values for revealDuration, then re-masks.
func (m Model) applyRevealBoth(msg revealBothMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	conflict := m.conflict.get()
	if conflict == nil {
		return m, nil
	}
	conflict.source = strings.TrimRight(string(msg.source), "\n")
	conflict.vault = strings.TrimRight(string(msg.vault), "\n")
	conflict.revealEnd = m.deps.Now().Add(revealDuration)
	clear(msg.source)
	clear(msg.vault)
	return m, tea.Tick(revealDuration, func(at time.Time) tea.Msg { return tickMsg{at} })
}

// updateConflict handles keys while the conflict modal is open. There is no
// overwrite-all key by design.
func (m Model) updateConflict(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	decision, ok := conflictKeys[msg.String()]
	if !ok {
		return m, nil
	}
	if decision == decisionReveal {
		return m, m.requestReveal()
	}
	conflict := m.conflict.get()
	m.conflict.set(nil)
	conflict.answer <- decision
	return m, nil
}

const decisionReveal = syncer.Decision(-1)

var conflictKeys = map[string]syncer.Decision{
	"o": syncer.DecisionOverwrite,
	"s": syncer.DecisionSkip,
	"q": syncer.DecisionQuit,
	"v": decisionReveal,
}

// requestReveal asks the syncer for both values without deciding.
func (m Model) requestReveal() tea.Cmd {
	conflict := m.conflict.get()
	return func() tea.Msg {
		source, vault, err := conflict.conflict.Reveal()
		msg := revealBothMsg{source: source, vault: vault, err: err}
		select {
		case conflict.reveal <- msg:
		default:
		}
		return nil
	}
}

// applySyncDone records the outcome and stops the spinner.
func (m Model) applySyncDone(msg syncDoneMsg) (tea.Model, tea.Cmd) {
	m.syncing = false
	m.notice = fmt.Sprintf("synced: %d changed · %d unchanged · %d skipped",
		msg.added, msg.unchanged, msg.skipped)
	return m, nil
}

func (m Model) applyCheckDone(msg checkDoneMsg) (tea.Model, tea.Cmd) {
	m.rows = msg.rows
	m.notice = "check complete"
	return m, nil
}

// syncView renders progress lines and the conflict modal when open.
func (m Model) syncView() string {
	head := m.styles.title.Render("Sync") + m.styles.dim.Render(" ─ 1Password → pass")
	lines := []string{head}
	lines = append(lines, m.syncLines...)
	if m.syncing && m.conflict.get() == nil {
		lines = append(lines, m.styles.dim.Render("  running…"))
	}
	if m.conflict.get() != nil {
		lines = append(lines, m.conflictModal())
	}
	return strings.Join(lines, "\n")
}

// conflictModal renders the prompt like the design mockup.
func (m Model) conflictModal() string {
	conflict := m.conflict.get().conflict
	lines := []string{
		"┌ Conflict · " + conflict.Secret.Pass + " ────────────",
		fmt.Sprintf("│ 1Password  %d chars", conflict.SourceLen),
		fmt.Sprintf("│ pass       %d chars", conflict.VaultLen),
	}
	lines = append(lines, m.conflictRevealLines()...)
	lines = append(lines,
		"│",
		"│ [o] overwrite pass   [s] skip   [v] reveal both for 10 s",
		"│ [q] stop sync",
		"└───────────────────────────────")
	return strings.Join(lines, "\n")
}

func (m Model) conflictRevealLines() []string {
	conflict := m.conflict.get()
	if conflict.revealEnd.IsZero() {
		return nil
	}
	if !m.deps.Now().Before(conflict.revealEnd) {
		return nil
	}
	return []string{
		"│ 1Password: " + conflict.source,
		"│ pass:      " + conflict.vault,
	}
}
