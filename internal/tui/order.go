package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// Reorder mode arranges the day on screen. o opens it, j and k select an
// item, J and K move it, and Ctrl+S saves the whole order at once, like
// every save. Esc leaves, asking first if anything moved. Nothing is saved
// until Ctrl+S, and while it is open the day is not read again, so nothing
// shifts underneath. The planner refuses the order if the day changed in
// the meantime.

// ordering is the open reorder mode: the order as it was saved, to put back
// on cancel, and whether anything moved.
type ordering struct {
	saved  []planner.ItemView
	moved  bool
	asking bool // whether to throw the new order away
}

func (m *model) openOrdering() {
	if !m.plannable() || m.busy() || len(m.state.View.Items) < 2 {
		return
	}
	if m.selected == "" {
		m.selected = m.state.View.Items[0].ID
	}
	m.ordering, m.details = &ordering{saved: append([]planner.ItemView(nil), m.state.View.Items...)}, false
}

// orderKey handles keys in reorder mode.
func (m *model) orderKey(key string) tea.Cmd {
	o := m.ordering
	if o.asking {
		switch key {
		case "y":
			m.state.View.Items, m.ordering = o.saved, nil
		case "n", "esc":
			o.asking = false
		}
		return nil
	}
	switch key {
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "J", "shift+down":
		m.shift(1)
	case "K", "shift+up":
		m.shift(-1)
	case "esc":
		if o.moved {
			o.asking = true
		} else {
			m.ordering = nil
		}
	case "ctrl+s":
		if !o.moved {
			m.ordering = nil
			return nil
		}
		var order []domain.ID
		for _, item := range m.state.View.Items {
			order = append(order, item.ID)
		}
		intent := planner.Intent{Operation: planner.Reorder, Selection: planner.Selection{Date: m.date}, Order: order}
		m.ordering, m.pending = nil, true
		return func() tea.Msg {
			r, err := m.client.Do(m.ctx, intent)
			return actionMsg{r, err}
		}
	}
	return nil
}

// shift moves the selected item one place up or down on screen.
func (m *model) shift(by int) {
	items := m.state.View.Items
	i := m.index()
	j := i + by
	if i < 0 || j < 0 || j >= len(items) {
		return
	}
	items = append([]planner.ItemView(nil), items...)
	items[i], items[j] = items[j], items[i]
	m.state.View.Items, m.ordering.moved = items, true
}

func (m *model) orderFooter() string {
	if m.ordering.asking {
		return " Throw away the new order?  yes [y]  no [n]"
	}
	return " move [J/K]  select [j/k]  save [ctrl+s]  cancel [esc]"
}
