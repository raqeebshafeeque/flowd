package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// actionMsg is the finished result of a change.
type actionMsg struct {
	result planner.Result
	err    error
}

// reconcileMsg is the answer to whether an unconfirmed change was saved.
type reconcileMsg struct {
	result planner.Result
	err    error
}

// trackKeys maps keys to the status actions they perform.
var trackKeys = map[string]planner.Operation{
	"f": planner.Finish,
	"s": planner.Skip,
	"u": planner.Unskip,
	"R": planner.Reopen,
}

// spaceAction is what space does for an item: start, pause or resume.
func spaceAction(status domain.Status) planner.Operation {
	switch status {
	case domain.Todo:
		return planner.Start
	case domain.InProgress:
		return planner.Pause
	case domain.Paused:
		return planner.Resume
	}
	return ""
}

// allowed lists the actions that make sense for an item right now, in the
// order the footer shows them.
func allowed(item planner.ItemView) []planner.Operation {
	switch item.Status {
	case domain.Todo:
		return []planner.Operation{planner.Start, planner.Skip}
	case domain.InProgress:
		return []planner.Operation{planner.Pause, planner.Finish, planner.Skip}
	case domain.Paused:
		return []planner.Operation{planner.Resume, planner.Finish, planner.Skip}
	case domain.Done:
		return []planner.Operation{planner.Reopen}
	case domain.Skipped:
		return []planner.Operation{planner.Unskip}
	}
	return nil
}

// busy is true while a change is saving or its outcome is unknown. No other
// change may start then, so nothing is ever sent twice.
func (m *model) busy() bool { return m.pending || m.unresolved != nil }

// track starts a status action on the selected item. Actions that do not fit
// the item's status are still sent, so the planner explains the refusal.
func (m *model) track(op planner.Operation) tea.Cmd {
	i := m.index()
	if i < 0 || op == "" || m.busy() {
		return nil
	}
	item := m.state.View.Items[i]
	intent := planner.Intent{Operation: op, ItemID: item.ID, Revision: m.state.Revisions[item.ID]}
	m.pending = true
	return func() tea.Msg {
		r, err := m.client.Do(m.ctx, intent)
		return actionMsg{r, err}
	}
}

// finished handles a change's result. Success is quiet: the next read shows
// it. A refusal pops up. An unknown outcome is reconciled before anything
// else can change.
func (m *model) finished(msg actionMsg) tea.Cmd {
	m.pending = false
	switch {
	case msg.result.Outcome == planner.Unconfirmed:
		r := msg.result
		m.unresolved = &r
		m.popup(errors.Join(planner.ErrStillUnknown, msg.err))
		return m.reconcile()
	case msg.err != nil && msg.result.Outcome != planner.Confirmed:
		m.popup(msg.err)
	}
	return m.read()
}

func (m *model) reconcile() tea.Cmd {
	pending := *m.unresolved
	return func() tea.Msg {
		r, err := m.client.Reconcile(m.ctx, pending)
		return reconcileMsg{r, err}
	}
}

// reconciled clears the unknown outcome once the saved history answers it.
// While it stays unknown, every tick asks again.
func (m *model) reconciled(msg reconcileMsg) tea.Cmd {
	if msg.err != nil {
		if !errors.Is(msg.err, planner.ErrStillUnknown) {
			m.popup(msg.err)
		}
		return nil
	}
	m.unresolved = nil
	return m.read()
}
