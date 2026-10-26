// Package tui is the interactive pvault interface: a vault browser, a sync
// view with conflict prompts, history with rollback and a mapping editor.
// All logic lives in Model so tests can drive Update without a terminal.
package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
	"github.com/atavares-klaviyo/pass-vault/internal/opcli"
	"github.com/atavares-klaviyo/pass-vault/internal/store"
	"github.com/atavares-klaviyo/pass-vault/internal/syncer"
)

// Deps wires the TUI to the rest of pvault.
type Deps struct {
	Mapping     func() (mapping.Mapping, error)
	MappingPath string
	Store       *store.Store
	Source      syncer.Source
	Items       ItemSource
	State       func() (syncer.State, error)
	SaveState   func(syncer.State) error
	Now         func() time.Time
	Editor      func(path string) *exec.Cmd
}

// ItemSource lists 1Password items and their fields, without values.
type ItemSource interface {
	ListItems(ctx context.Context) ([]opcli.Item, error)
	ItemFields(ctx context.Context, id string) (opcli.Item, error)
}

// Run opens the TUI. It is a thin shell: all logic is in Model.
func Run(deps Deps) error {
	program := tea.NewProgram(NewModel(deps))
	_, err := program.Run()
	return err
}

type view int

const (
	viewVault view = iota
	viewSync
	viewHistory
	viewMapping
	viewCount
)

// viewNames label the tab bar, in view order.
var viewNames = [viewCount]string{"Vault", "Sync", "History", "Mapping"}

// promptKind is the active modal input, if any.
type promptKind int

const (
	promptNone promptKind = iota
	promptNewName
	promptNewValue
	promptDelete
	promptRollback
	promptAddName
	promptAddEnv
)

// pickerStep tracks the add-from-1Password picker stages.
type pickerStep int

const (
	pickerItems pickerStep = iota
	pickerFields
)

// revealState holds a decrypted value until the timer re-masks it. It never
// leaves the model.
type revealState struct {
	name     string
	value    string
	deadline time.Time
}

// Model is the whole TUI state.
type Model struct {
	deps    Deps
	width   int
	height  int
	active  view
	styles  styles
	help    help.Model
	helpAll bool
	notice  string
	err     error
	opOK    bool

	mapping mapping.Mapping
	state   syncer.State
	rows    []syncer.StatusRow
	cursor  int
	history []store.Revision
	reveal  *revealState

	prompt  promptKind
	input   string
	pending string

	syncing   bool
	syncLines []string
	conflict  *conflictBox

	revisions    []store.Revision
	syncTags     []string
	revCursor    int
	planRestored []string
	planRemoved  []string
	planTarget   string

	mapCursor    int
	mapErrors    string
	picker       pickerStep
	pickerItems  []opcli.Item
	pickerCursor int
	pickerFields []opcli.Field
	fieldCursor  int
	pickerRef    string

	program *tea.Program
	// conflictHook delivers conflictMsg without a live program; tests set it
	// before the sync goroutine starts, so it needs no lock.
	conflictHook func(tea.Msg)
}

// NewModel builds the initial model; loading starts in Init.
func NewModel(deps Deps) Model {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return Model{
		deps:     deps,
		styles:   newStyles(true),
		help:     help.New(),
		opOK:     true,
		conflict: &conflictBox{},
	}
}

// Init starts loading the mapping, state, store and op health.
func (m Model) Init() tea.Cmd {
	return tea.Batch(loadCmd(m.deps), tea.RequestBackgroundColor)
}

// Update routes messages to the matching handler.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, ok := m.updateData(msg); ok {
		return next, cmd
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		return m.updateKey(key)
	}
	return m, nil
}

// updateData handles plain data messages; async messages with their own
// apply method go to updateAsync. handled=false means unknown message.
func (m Model) updateData(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
	case tea.BackgroundColorMsg:
		m.styles = newStyles(msg.IsDark())
	case historyMsg:
		m.history = msg.revisions
	case revisionsMsg:
		m.revisions, m.syncTags = msg.revisions, msg.tags
	case itemsMsg:
		m.pickerItems, m.pickerCursor = msg.items, 0
	case fieldsMsg:
		m.pickerFields, m.fieldCursor = msg.item.Fields, 0
	case errMsg:
		m.err, m.syncing = msg.err, false
		m.conflict.set(nil)
	default:
		return m.updateDone(msg)
	}
	return m, nil, true
}

// updateDone handles messages that end an operation and reload.
func (m Model) updateDone(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appendedMsg:
		m.picker = pickerItems
		m.notice = "added " + msg.name
		return m, loadCmd(m.deps), true
	case rollbackDoneMsg:
		m.notice = "rolled back to " + msg.hash
		return m, loadCmd(m.deps), true
	case mutatedMsg:
		m.notice = msg.note
		return m, loadCmd(m.deps), true
	case planMsg:
		next, cmd := m.applyPlan(msg)
		return next, cmd, true
	}
	return m.updateAsync(msg)
}

// updateAsync handles messages that carry their own apply method.
func (m Model) updateAsync(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	handled := true
	var next tea.Model
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case loadedMsg:
		next, cmd = m.applyLoaded(msg)
	case revealMsg:
		next, cmd = m.applyReveal(msg)
	case tickMsg:
		next, cmd = m.applyTick(msg)
	case conflictMsg:
		next, cmd = m.openConflict(msg)
	case revealBothMsg:
		next, cmd = m.applyRevealBoth(msg)
	case syncDoneMsg:
		next, cmd = m.applySyncDone(msg)
	case checkDoneMsg:
		next, cmd = m.applyCheckDone(msg)
	case editorDoneMsg:
		next, cmd = m.applyEditorDone(msg)
	default:
		handled = false
	}
	return next, cmd, handled
}

// updateKey handles keys by mode: modal input first, then view switch,
// then the active view.
func (m Model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.prompt != promptNone {
		return m.updatePrompt(msg)
	}
	if m.conflict.get() != nil {
		return m.updateConflict(msg)
	}
	if next, cmd, ok := m.updateGlobalKey(msg); ok {
		return next, cmd
	}
	return m.updateViewKey(msg)
}

// updateGlobalKey handles keys that work in every view.
func (m Model) updateGlobalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "q", "ctrl+c":
		if m.syncing {
			// Quitting would orphan the sync goroutine, which blocks on the
			// conflict answer channel forever.
			return m, nil, true
		}
		m.clearReveal()
		return m, tea.Quit, true
	case "tab", "right":
		next, cmd := m.switchView((m.active + 1) % viewCount)
		return next, cmd, true
	case "shift+tab", "left":
		next, cmd := m.switchView((m.active + viewCount - 1) % viewCount)
		return next, cmd, true
	case "?":
		m.helpAll = !m.helpAll
		m.help.ShowAll = m.helpAll
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) updateViewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.active {
	case viewVault:
		return m.updateVault(msg)
	case viewSync:
		if msg.String() == "s" {
			return m.startSync()
		}
	case viewHistory:
		return m.updateHistory(msg)
	case viewMapping:
		return m.updateMapping(msg)
	}
	return m, nil
}

// switchView moves to next and clears any revealed secret.
func (m Model) switchView(next view) (tea.Model, tea.Cmd) {
	m.active = next
	m.clearReveal()
	m.notice, m.err = "", nil
	if next == viewHistory {
		return m, revisionsCmd(m.deps.Store)
	}
	return m, nil
}

func (m *Model) clearReveal() {
	m.reveal = nil
}

// selectedRow returns the row under the vault cursor, if any.
func (m Model) selectedRow() (syncer.StatusRow, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return syncer.StatusRow{}, false
	}
	return m.rows[m.cursor], true
}

// counts summarizes the rows for the header.
func (m Model) counts() (mapped, unmanaged, orphans int) {
	for _, row := range m.rows {
		switch row.Kind {
		case syncer.KindMapped, syncer.KindMappedMissing:
			mapped++
		case syncer.KindUnmanaged:
			unmanaged++
		case syncer.KindOrphan:
			orphans++
		}
	}
	return mapped, unmanaged, orphans
}

// header renders the top line like the design mockup.
func (m Model) header() string {
	mapped, unmanaged, orphans := m.counts()
	opMark := m.styles.ok.Render("op ✓")
	if !m.opOK {
		opMark = m.styles.bad.Render("op ✕")
	}
	title := m.styles.title.Render("pvault")
	dim := m.styles.dim.Render("─ " + m.deps.Store.Dir + " ─")
	summary := fmt.Sprintf("%d mapped · %d unmanaged · %d orphan", mapped, unmanaged, orphans)
	return fmt.Sprintf(" %s %s %s  %s", title, dim, summary, opMark)
}

// tabBar renders one tab per view with the active one highlighted.
func (m Model) tabBar() string {
	tabs := make([]string, viewCount)
	for i, name := range viewNames {
		style := m.styles.tab
		if view(i) == m.active {
			style = m.styles.activeTab
		}
		tabs[i] = style.Render(name)
	}
	return " " + strings.Join(tabs, " ")
}

// footer renders the key hints for whatever has focus.
func (m Model) footer() string {
	return " " + m.help.View(m.keyMap())
}

// chromeHeight counts the header, tab bar, footer, status and divider lines.
const chromeHeight = 6

// bodyHeight is the space left for the active view.
func (m Model) bodyHeight() int {
	return max(1, m.height-chromeHeight)
}

// render frames body between the header and footer and fills the screen.
func (m Model) render(body string) string {
	rule := m.styles.dim.Render(strings.Repeat("─", m.width))
	if m.height > 0 {
		body = lipgloss.NewStyle().Height(m.bodyHeight()).MaxHeight(m.bodyHeight()).Render(body)
	}
	return strings.Join([]string{m.header(), m.tabBar(), rule, body, rule, m.statusLine(), m.footer()}, "\n")
}

func (m Model) statusLine() string {
	if m.err != nil {
		return " " + m.styles.bad.Render("error: "+m.err.Error())
	}
	if m.notice != "" {
		return " " + m.styles.dim.Render(m.notice)
	}
	return ""
}

// age renders t relative to now as a short duration, or "never". It mirrors
// internal/cli's unexported formatAge.
func age(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	delta := now.Sub(t)
	if delta < 0 {
		delta = 0
	}
	switch {
	case delta < time.Minute:
		return fmt.Sprintf("%ds", int(delta.Seconds()))
	case delta < time.Hour:
		return fmt.Sprintf("%dm", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh", int(delta.Hours()))
	default:
		return fmt.Sprintf("%dd", int(delta.Hours()/24))
	}
}

// slugify lowercases title and turns non-alphanumerics into dashes.
func slugify(title string) string {
	var out []rune
	dash := false
	for _, r := range strings.ToLower(title) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			out, dash = append(out, r), false
			continue
		}
		if !dash && len(out) > 0 {
			out, dash = append(out, '-'), true
		}
	}
	return strings.TrimRight(string(out), "-")
}
