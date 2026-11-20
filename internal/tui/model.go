package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
	"github.com/raqeebshafeeque/flowd/internal/store"
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
	calendar   *calendar
	carrying   *carryPanel // the carry panel, when it is open
	ordering   *ordering   // reorder mode, when it is on
	correcting domain.ID   // the item a mark done question is about
	moreHints  bool        // the footer shows every hint, on more rows
	date       string      // the date on screen, or "" for today

	review       bool // the review report replaces the list
	reviewOffset int
	help         bool // the help page replaces the list
	helpOffset   int
	lastError    *planner.Feedback // kept for help after its popup expires
	deleting     domain.ID         // the item a delete question is about
	caret        *tea.Cursor       // where the terminal cursor goes, if anywhere

	signature store.Signature // the file when the day on screen was read
	signed    bool
	lastRead  time.Time
}

func newModel(ctx context.Context, client planner.Client, zone *time.Location, now func() time.Time, noColor bool) *model {
	return &model{ctx: ctx, client: client, zone: zone, now: now, style: style{noColor: noColor}}
}

type (
	stateMsg struct {
		state     planner.State
		err       error
		date      string          // which date was asked for
		signature store.Signature // the file as it was just after the read
		signed    bool
	}
	tickMsg struct{}
)

func (m *model) Init() tea.Cmd { return tea.Batch(m.read(), tick()) }

// read loads the day in the background. The database is only open for the
// length of the read.
func (m *model) read() tea.Cmd {
	return func() tea.Msg {
		date := m.date
		state, err := m.client.Read(m.ctx, planner.Selection{Date: date})
		// The signature is taken after the read, because closing the database
		// folds SQLite's WAL into the file and changes it. A write by someone
		// else in the moment between is caught by the thirty second read.
		signature, sigErr := m.client.Signature()
		return stateMsg{state, err, date, signature, sigErr == nil}
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
	case leftoversMsg:
		m.receiveLeftovers(msg)
	case tickMsg:
		cmds := []tea.Cmd{tick()}
		if m.readDue() {
			cmds = append(cmds, m.read())
		} else {
			m.recount()
		}
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
	if msg.date != m.date {
		return // an answer for a date no longer on screen
	}
	if m.ordering != nil {
		return // the day being arranged is not replaced until it is saved
	}
	m.err = msg.err
	if msg.err != nil {
		return
	}
	previous := m.index()
	m.state, m.loaded = msg.state, true
	m.signature, m.signed, m.lastRead = msg.signature, msg.signed, m.now()
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
	// Any key but . folds the extra hints away again.
	if msg.String() != "." {
		m.moreHints = false
	}
	if m.form != nil {
		return m.formKey(msg)
	}
	if m.deleting != "" {
		return m.deleteKey(msg.String())
	}
	if m.correcting != "" {
		return m.correctKey(msg.String())
	}
	if m.help {
		m.helpKey(msg.String())
		return nil
	}
	if m.calendar != nil {
		return m.calendarKey(msg)
	}
	if m.carrying != nil {
		return m.carryKey(msg.String())
	}
	if m.ordering != nil {
		return m.orderKey(msg.String())
	}
	if m.review {
		if msg.String() == "q" {
			return tea.Quit
		}
		m.reviewKey(msg.String())
		return nil
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
	case "C":
		return m.openCarry()
	case "o":
		m.openOrdering()
	case ".":
		m.moreHints = !m.moreHints
	case "n":
		if m.plannable() {
			m.openNew()
		}
	case "e":
		if m.plannable() {
			m.openEdit()
		}
	case "c":
		m.openCalendar()
	case "r":
		m.openReview()
	case "d":
		m.askDelete()
	case "?":
		m.toggleHelp()
	case "t":
		if !m.busy() {
			return m.show(m.today())
		}
	case "space":
		if i := m.index(); i >= 0 && m.trackable() {
			return m.track(spaceAction(m.state.View.Items[i].Status))
		}
	default:
		if msg.String() == "f" && !m.trackable() && m.correctable() {
			m.correcting = m.selected
			return nil
		}
		if op, ok := trackKeys[msg.String()]; ok && m.trackable() {
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

// trackable says whether items on screen can be started, finished and so on:
// only today's can.
func (m *model) trackable() bool { return m.loaded && m.viewing() == m.today() }

// plannable says whether items on screen can be added or edited: today's and
// later days' can.
func (m *model) plannable() bool { return m.loaded && m.viewing() >= m.today() }

// refreshInterval is the longest the workspace goes without reading, even
// when the file looks unchanged, in case a clock or filesystem hides a change.
const refreshInterval = 30 * time.Second

// readDue says whether the next tick must read storage. Reading is only
// needed when the file may have changed, when the day on screen moved on at
// midnight, or after the refresh interval.
func (m *model) readDue() bool {
	if !m.loaded || !m.signed || m.err != nil || m.state.Day.Date != m.viewing() || m.now().Sub(m.lastRead) >= refreshInterval {
		return true
	}
	current, err := m.client.Signature()
	return err != nil || current != m.signature
}

// recount measures the day on screen again at the current time, from what is
// already in memory, so running work keeps counting without a read.
func (m *model) recount() {
	if !m.loaded {
		return
	}
	items := make([]domain.Item, 0, len(m.state.View.Items))
	for _, item := range m.state.View.Items {
		items = append(items, item.Item)
	}
	if view, err := planner.NewView(m.state.Day, items, m.state.History, m.now()); err == nil {
		m.state.View = view
	}
}
