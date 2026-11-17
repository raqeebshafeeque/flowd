package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// Minimum size of a usable workspace.
const (
	minWidth  = 80
	minHeight = 24
)

// model is the whole workspace state.
type model struct {
	ctx    context.Context
	client planner.Client
	zone   *time.Location // for showing times
	now    func() time.Time
	style  style

	width, height int

	loaded   bool
	state    planner.State
	err      error
	selected domain.ID
	details  bool

	pending    bool            // a change is being saved
	unresolved *planner.Result // a change whose outcome is not known yet
	notice     *notice
	form       *form
	caret      *tea.Cursor // where the terminal cursor goes, if anywhere
}

func newModel(ctx context.Context, client planner.Client, zone *time.Location, now func() time.Time, noColor bool) *model {
	return &model{ctx: ctx, client: client, zone: zone, now: now, style: style{noColor: noColor}}
}

type (
	stateMsg struct {
		state planner.State
		err   error
	}
	tickMsg struct{}
)

func (m *model) Init() tea.Cmd { return tea.Batch(m.read(), tick()) }

// read loads the day in the background. The database is only open for the
// length of the read.
func (m *model) read() tea.Cmd {
	return func() tea.Msg {
		state, err := m.client.Read(m.ctx, planner.Selection{})
		return stateMsg{state, err}
	}
}

// tick refreshes once a second so running work and the clock stay current.
func tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *model) usable() bool { return m.width >= minWidth && m.height >= minHeight }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case stateMsg:
		m.receive(msg)
	case tickMsg:
		cmds := []tea.Cmd{m.read(), tick()}
		if m.unresolved != nil {
			cmds = append(cmds, m.reconcile())
		}
		return m, tea.Batch(cmds...)
	case actionMsg:
		return m, m.finished(msg)
	case reconcileMsg:
		return m, m.reconciled(msg)
	case tea.KeyPressMsg:
		return m, m.key(msg)
	case tea.PasteMsg:
		if m.usable() {
			m.paste(msg.Content)
		}
	}
	return m, nil
}

// receive takes a fresh read. A failed read keeps showing the last good day
// along with the error.
func (m *model) receive(msg stateMsg) {
	m.err = msg.err
	if msg.err != nil {
		return
	}
	previous := m.index()
	m.state, m.loaded = msg.state, true
	items := m.state.View.Items
	switch {
	case len(items) == 0:
		m.selected, m.details = "", false
	case m.index() < 0:
		// The selected item is gone: keep the same position if possible.
		m.selected = items[min(max(previous, 0), len(items)-1)].ID
	}
}

// index is the position of the selected item, or -1.
func (m *model) index() int {
	for i, item := range m.state.View.Items {
		if item.ID == m.selected {
			return i
		}
	}
	return -1
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "ctrl+c" {
		return tea.Quit
	}
	if !m.usable() {
		return nil // nothing happens that cannot be seen
	}
	if m.form != nil {
		return m.formKey(msg)
	}
	switch msg.String() {
	case "q":
		return tea.Quit
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "enter", "v":
		m.details = m.selected != "" && !m.details
	case "esc":
		m.details = false
	case "n":
		m.openNew()
	case "e":
		m.openEdit()
	case "space":
		if i := m.index(); i >= 0 {
			return m.track(spaceAction(m.state.View.Items[i].Status))
		}
	default:
		if op, ok := trackKeys[msg.String()]; ok {
			return m.track(op)
		}
	}
	return nil
}

func (m *model) move(delta int) {
	items := m.state.View.Items
	if len(items) == 0 {
		return
	}
	i := min(max(m.index()+delta, 0), len(items)-1)
	m.selected = items[i].ID
}
