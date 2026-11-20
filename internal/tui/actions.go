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

// spaceAction is what space does for an item: pause it when it runs, and
// otherwise start it, which also resumes a paused item and reopens a done one.
func spaceAction(status domain.Status) planner.Operation {
	switch status {
	case domain.InProgress:
		return planner.Pause
	case domain.Todo, domain.Paused, domain.Done:
		return planner.Start
	}
	return ""
}

// allowed lists the actions that make sense for an item right now, in the
// order the footer shows them.
func allowed(item planner.ItemView) []planner.Operation {
	switch item.Status {
	case domain.Todo, domain.Done:
		return []planner.Operation{planner.Start}
	case domain.InProgress:
		return []planner.Operation{planner.Pause, planner.Finish}
	case domain.Paused:
		return []planner.Operation{planner.Start, planner.Finish}
	}
	return nil
}

// busy is true while a change is saving. No other change may start then, so
// nothing is ever sent twice.
func (m *model) busy() bool { return m.pending }

// track starts a status action on the selected item. Actions that do not fit
// the item's status are still sent, so the planner explains the refusal.
func (m *model) track(op planner.Operation) tea.Cmd {
	i := m.index()
	if i < 0 || op == "" || m.busy() {
		return nil
	}
	item := m.state.View.Items[i]
	intent := planner.Intent{Operation: op, Selection: planner.Selection{Date: m.date}, ItemID: item.ID, Revision: m.state.Revisions[item.ID]}
	m.pending = true
	return func() tea.Msg {
		r, err := m.client.Do(m.ctx, intent)
		return actionMsg{r, err}
	}
}

// finished handles a change's result. Success is quiet: the next read shows
// it. A refusal pops up, and so does an unknown outcome, which the next read
// settles.
func (m *model) finished(msg actionMsg) tea.Cmd {
	m.pending = false
	formError := false
	if m.form != nil && m.form.saving {
		m.formFinished(msg)
		formError = m.form != nil && m.form.errors != [2]string{}
	}
	switch {
	case formError:
	case msg.result.Outcome == planner.Unconfirmed:
		m.popup(errors.Join(planner.ErrStillUnknown, msg.err))
	case msg.err != nil && msg.result.Outcome != planner.Confirmed:
		m.popup(msg.err)
	}
	return m.read()
}

// askDelete asks before deleting the selected item. Deleting hides it from
// the day and keeps its history.
func (m *model) askDelete() {
	if m.index() >= 0 && m.plannable() && !m.busy() {
		m.deleting = m.selected
	}
}

// deleteKey answers the question. Only y deletes.
func (m *model) deleteKey(key string) tea.Cmd {
	switch key {
	case "y":
		id := m.deleting
		m.deleting = ""
		if id != m.selected || m.index() < 0 {
			return nil
		}
		return m.track(planner.Delete)
	case "n", "esc":
		m.deleting = ""
	}
	return nil
}
