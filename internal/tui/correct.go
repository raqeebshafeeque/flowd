package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// On a past day, f puts right a forgotten finish: it asks first, and only y
// marks the selected item done. It works on items left incomplete or
// skipped in the last seven days. Its work stays what was tracked.

// correctable says whether the selected item can be marked done later.
func (m *model) correctable() bool {
	i := m.index()
	if i < 0 || !m.loaded || m.busy() {
		return false
	}
	day, today := m.viewing(), m.today()
	t, _ := time.Parse("2006-01-02", today)
	earliest := t.AddDate(0, 0, -domain.CorrectDays).Format("2006-01-02")
	return day < today && day >= earliest && m.state.View.Items[i].Correctable()
}

// correctKey answers the question. Only y marks the item done.
func (m *model) correctKey(key string) tea.Cmd {
	switch key {
	case "y":
		id := m.correcting
		m.correcting = ""
		if id != m.selected || m.index() < 0 {
			return nil
		}
		return m.track(planner.MarkDone)
	case "n", "esc":
		m.correcting = ""
	}
	return nil
}

// correctQuestion asks before marking an item done later, with the title
// shortened so the answers always show.
func (m *model) correctQuestion(width int) string {
	title, was := "", "incomplete"
	if i := m.index(); i >= 0 {
		item := m.state.View.Items[i]
		title, was = item.Title, planner.Words(item.Status)
	}
	answers := "  yes [y]  no [n]"
	rest := "\" done? It was left " + was + "."
	room := width - ansi.StringWidth(answers) - ansi.StringWidth(" Mark \""+rest)
	return fit(" Mark \""+ansi.Truncate(title, max(1, room), "…")+rest+answers, width)
}
