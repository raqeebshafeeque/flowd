package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// The carry panel lists the incomplete and skipped items from the seven
// days before the day on screen, with a box to tick for each. Space ticks
// one, a ticks or clears them all, and Enter copies what is ticked onto the
// day, or the highlighted item when nothing is ticked. Esc closes it. The
// footer offers carry [C] whenever the day can be planned.

// carryPanel is the open carry panel.
type carryPanel struct {
	loaded bool
	err    error
	items  []planner.Leftover
	picked []bool
	cursor int
	offset int
}

// leftoversMsg is what can be carried, as read.
type leftoversMsg struct {
	date  string
	items []planner.Leftover
	err   error
}

func (m *model) readLeftovers() tea.Cmd {
	date := m.date
	return func() tea.Msg {
		items, err := m.client.Leftovers(m.ctx, planner.Selection{Date: date})
		return leftoversMsg{date, items, err}
	}
}

func (m *model) openCarry() tea.Cmd {
	if !m.plannable() || m.busy() {
		return nil
	}
	m.carrying, m.details = &carryPanel{}, false
	return m.readLeftovers()
}

// receiveLeftovers fills the panel, if it is still open on the same day.
func (m *model) receiveLeftovers(msg leftoversMsg) {
	if msg.date != m.date || m.carrying == nil {
		return
	}
	m.carrying.loaded, m.carrying.err = true, msg.err
	m.carrying.items, m.carrying.picked = msg.items, make([]bool, len(msg.items))
}

// carryKey handles keys while the carry panel is open.
func (m *model) carryKey(key string) tea.Cmd {
	c := m.carrying
	if key == "esc" {
		m.carrying = nil
		return nil
	}
	if !c.loaded || len(c.items) == 0 {
		return nil
	}
	switch key {
	case "j", "down":
		c.cursor = min(c.cursor+1, len(c.items)-1)
	case "k", "up":
		c.cursor = max(c.cursor-1, 0)
	case "space":
		c.picked[c.cursor] = !c.picked[c.cursor]
	case "a":
		all := true
		for _, p := range c.picked {
			all = all && p
		}
		for i := range c.picked {
			c.picked[i] = !all
		}
	case "enter":
		if m.busy() {
			return nil
		}
		ids := c.chosen()
		intent := planner.Intent{Operation: planner.Carry, Selection: planner.Selection{Date: m.date}, Sources: ids}
		m.carrying, m.pending = nil, true
		return func() tea.Msg {
			r, err := m.client.Do(m.ctx, intent)
			return actionMsg{r, err}
		}
	}
	return nil
}

// chosen is what Enter carries: the ticked items, or the highlighted one.
func (c *carryPanel) chosen() []domain.ID {
	var ids []domain.ID
	for i, on := range c.picked {
		if on {
			ids = append(ids, c.items[i].Item.ID)
		}
	}
	if len(ids) == 0 {
		ids = append(ids, c.items[c.cursor].Item.ID)
	}
	return ids
}

func (c *carryPanel) ticked() int {
	n := 0
	for _, on := range c.picked {
		if on {
			n++
		}
	}
	return n
}

// carryHeight is how many rows the panel takes: its heading, a gap and as
// many items as there are, up to half the body.
func (m *model) carryHeight(body int) int {
	items := 1
	if m.carrying != nil && len(m.carrying.items) > 0 {
		items = len(m.carrying.items)
	}
	return min(2+items, max(4, body/2))
}

// carryRows draws the panel: a heading, then one row per item with its tick
// box, title, the day it is from, its duration and its status, in aligned
// columns, scrolled to keep the highlighted item in view.
func (m *model) carryRows(width, height int) []string {
	c := m.carrying
	rows := []string{" " + m.style.paint("heading", "Carry to "+m.dayName()), ""}
	switch {
	case c.err != nil:
		return append(rows, " "+m.style.paint("error", planner.Describe(c.err).Summary))
	case !c.loaded:
		return append(rows, " "+m.style.paint("muted", "Reading…"))
	case len(c.items) == 0:
		return append(rows, " "+m.style.paint("muted", fmt.Sprintf("Nothing to carry from the last %d days.", domain.CarryDays)))
	}
	visible := max(1, height-2)
	c.offset = min(max(c.offset, c.cursor-visible+1), c.cursor)
	titleWidth, durationWidth, statusWidth := 0, 0, 0
	for _, it := range c.items {
		duration, _ := domain.FormatDuration(it.Item.DurationMinutes)
		titleWidth = max(titleWidth, ansi.StringWidth(it.Item.Title))
		durationWidth = max(durationWidth, len(duration))
		statusWidth = max(statusWidth, len(planner.Label(it.Item.Status)))
	}
	const dateWidth = len("Wed 18 Nov")
	right := 2 + dateWidth + 2 + durationWidth + 2 + statusWidth + 1
	// The title takes the room left, so the status ends at the right edge
	// like it does in the list above.
	titleWidth = max(12, width-1-6-right)
	for i := c.offset; i < len(c.items) && i < c.offset+visible; i++ {
		it := c.items[i]
		box := "[ ]"
		if c.picked[i] {
			box = "[x]"
		}
		name := ansi.Truncate(it.Item.Title, titleWidth, "…")
		left := "  " + box + " " + name + strings.Repeat(" ", titleWidth-ansi.StringWidth(name))
		if i == c.cursor {
			left = m.style.paint("selected", "> "+box+" "+name) + strings.Repeat(" ", titleWidth-ansi.StringWidth(name))
		}
		day, _ := time.Parse("2006-01-02", it.Date)
		duration, _ := domain.FormatDuration(it.Item.DurationMinutes)
		status := planner.Label(it.Item.Status)
		row := " " + left + "  " + m.style.paint("muted", day.Format("Mon 2 Jan")) + strings.Repeat(" ", dateWidth-len(day.Format("Mon 2 Jan"))) +
			"  " + strings.Repeat(" ", durationWidth-len(duration)) + duration + "  " + strings.Repeat(" ", statusWidth-len(status)) + m.style.status(it.Item.Status, status)
		rows = append(rows, row)
	}
	return rows
}

// carryFooter offers what the panel can do, counting what Enter carries.
func (m *model) carryFooter() string {
	c := m.carrying
	if !c.loaded || len(c.items) == 0 {
		return " cancel [esc]"
	}
	n := c.ticked()
	if n == 0 {
		n = 1
	}
	return fmt.Sprintf(" pick [space]  all [a]  move [j/k]  carry %d [enter]  cancel [esc]", n)
}

// dayName is the day on screen as the panel names it.
func (m *model) dayName() string {
	if m.viewing() == m.today() {
		return "today"
	}
	d, _ := time.Parse("2006-01-02", m.viewing())
	return d.Format("Mon 2 Jan")
}
