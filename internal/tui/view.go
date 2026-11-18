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

// View draws the whole screen. Every row is exactly the window width, so a
// resize never leaves stray characters behind.
func (m *model) View() tea.View {
	var rows []string
	m.caret = nil
	if m.usable() {
		rows = m.frame()
	} else {
		rows = m.suspended()
	}
	v := tea.NewView(strings.Join(rows, "\n"))
	v.AltScreen = true
	if m.usable() {
		v.Cursor = m.caret
		if caret := m.calendarCaret(); caret != nil {
			v.Cursor = caret
		}
	}
	return v
}

// frame is the header, the body and the footer inside one border.
func (m *model) frame() []string {
	inner := m.width - 2
	body := m.height - 6
	rows := []string{m.border("┌", "┐"), m.side(m.header(inner)), m.border("├", "┤")}
	var content []string
	caretRow, caretCol := -1, 0
	if m.help {
		content = m.helpBody(inner, body)
	} else if m.review && m.loaded {
		content = m.reviewBody(inner, body)
	} else if m.form != nil {
		panel := detailsHeight(body)
		list := body - panel - 1
		content = append(m.list(inner, list), m.style.paint("border", strings.Repeat("─", inner)))
		rows, row, col := m.formRows(inner, panel)
		content = append(content, rows...)
		caretRow, caretCol = list+1+row, col
	} else if m.calendar != nil {
		panel := min(calendarHeight, body-3)
		content = append(m.list(inner, body-panel-1), m.style.paint("border", strings.Repeat("─", inner)))
		content = append(content, pad(m.calendarRows(inner), panel)...)
	} else if m.details {
		detailRows := detailsHeight(body)
		content = append(m.list(inner, body-detailRows-1), m.style.paint("border", strings.Repeat("─", inner)))
		content = append(content, m.detailRows(inner, detailRows)...)
	} else {
		content = m.list(inner, body)
	}
	if popup := m.noticeRows(inner); popup != nil {
		content = append(content[:len(content)-len(popup)], popup...)
	}
	for _, row := range content {
		rows = append(rows, m.side(row))
	}
	m.caret = nil
	if caretRow >= 0 && !m.form.discard && caretRow < len(content) {
		m.caret = tea.NewCursor(1+caretCol, 3+caretRow)
		m.caret.Shape = tea.CursorBar
	}
	return append(rows, m.border("├", "┤"), m.side(m.footer(inner)), m.border("└", "┘"))
}

// detailsHeight is about a quarter of the body, never less than nine rows.
func detailsHeight(body int) int { return min(max(9, body/4+1), body-3) }

func (m *model) border(left, right string) string {
	return m.style.paint("border", left+strings.Repeat("─", m.width-2)+right)
}

func (m *model) side(row string) string {
	bar := m.style.paint("border", "│")
	return bar + fit(row, m.width-2) + bar
}

// fit pads or cuts a row to exactly width cells.
func fit(row string, width int) string {
	if w := ansi.StringWidth(row); w > width {
		return ansi.Truncate(row, width, "…")
	} else {
		return row + strings.Repeat(" ", width-w)
	}
}

// header shows the brand, the day being viewed when it is not today, and the
// current local date and time.
func (m *model) header(width int) string {
	now := m.now().In(m.zone)
	left := " " + m.style.paint("brand", "flowd")
	if day := m.viewing(); day != now.Format("2006-01-02") {
		left += "   Viewing " + day
	}
	right := now.Format("Mon 2 Jan 2006  15:04:05") + " "
	return spread(left, right, width)
}

// spread puts left and right at the two ends of a row.
func spread(left, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return fit(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// list shows one row per item, keeping the selected item in view.
func (m *model) list(width, height int) []string {
	rows := make([]string, 0, height)
	switch {
	case !m.loaded && m.err == nil:
		rows = append(rows, " Loading…")
	case !m.loaded:
		rows = append(rows, m.errorRows()...)
	case len(m.state.View.Items) == 0 && m.plannable():
		rows = append(rows, "", " Nothing planned for this day.", m.style.paint("muted", " Press n to add an item."))
	case len(m.state.View.Items) == 0:
		rows = append(rows, "", " Nothing was planned for this day.")
	default:
		items := m.state.View.Items
		first := max(0, m.index()-height+1)
		for i := first; i < len(items) && len(rows) < height; i++ {
			rows = append(rows, m.itemRow(items[i], width))
		}
	}
	if m.loaded && m.err != nil {
		rows = append(rows[:min(len(rows), max(0, height-2))], m.errorRows()...)
	}
	for len(rows) < height {
		rows = append(rows, "")
	}
	return rows[:height]
}

func (m *model) errorRows() []string {
	f := planner.Describe(m.err)
	return []string{m.style.paint("error", " "+f.Summary), m.style.paint("muted", " "+f.Hint)}
}

// itemRow is "> title ... work / duration  STATUS".
func (m *model) itemRow(item planner.ItemView, width int) string {
	marker := "  "
	title := item.Title
	if item.ID == m.selected {
		marker = m.style.paint("selected", "> ")
		title = m.style.paint("selected", title)
	}
	// Work and status sit at the right edge, the status in a fixed width
	// column so every row's timing ends in the same place.
	label := planner.Label(item.Status)
	right := planner.Effort(item) + "  " + strings.Repeat(" ", max(0, 11-len(label))) + m.style.status(item.Status, label) + " "
	room := width - ansi.StringWidth(right) - ansi.StringWidth(marker) - 1
	return spread(" "+marker+ansi.Truncate(title, room, "…"), right, width)
}

// detailRows shows the selected item in full: title, status and every
// measurement and time.
func (m *model) detailRows(width, height int) []string {
	rows := []string{" " + m.style.paint("heading", "Details"), " " + m.style.paint("border", strings.Repeat("─", width-2))}
	i := m.index()
	if i < 0 {
		return pad(rows, height)
	}
	item := m.state.View.Items[i]
	for j, line := range wrap(item.Title, width-15) {
		label := ""
		if j == 0 {
			label = "Title"
		}
		rows = append(rows, fmt.Sprintf(" %-12s %s", label, line))
	}
	duration, _ := domain.FormatDuration(item.DurationMinutes)
	fields := [][2]string{
		{"Status", m.style.status(item.Status, planner.Label(item.Status))},
		{"Duration", duration},
		{"Work", planner.Span(item.WorkNS)},
		{"Elapsed", planner.Elapsed(item)},
		{"Started At", m.instant(item.StartedAt)},
		{"Finished At", m.instant(item.FinishedAt)},
	}
	for _, f := range fields {
		rows = append(rows, fmt.Sprintf(" %-12s %s", f[0], f[1]))
	}
	return pad(rows, height)
}

func pad(rows []string, height int) []string {
	for len(rows) < height {
		rows = append(rows, "")
	}
	return rows[:height]
}

func (m *model) instant(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.In(m.zone).Format("2 Jan 2006 15:04:05")
}

// footer lists the keys that do something right now.
func (m *model) footer(width int) string {
	switch {
	case m.unresolved != nil:
		return fit(" Checking whether the last change was saved…", width)
	case m.form != nil:
		return fit(m.formFooter(), width)
	case m.deleting != "":
		return m.deleteQuestion(width)
	case m.help:
		return fit(" scroll [j/k]  back [esc]", width)
	case m.calendar != nil:
		return fit(m.calendarFooter(), width)
	case m.review:
		return fit(" scroll [j/k]  back [esc]  quit [q]", width)
	case m.pending:
		return fit(" Saving…", width)
	}
	type hint struct {
		text string
		keep int // higher is dropped later when the row is too narrow
	}
	var hints []hint
	if i := m.index(); i >= 0 {
		if m.trackable() {
			for _, op := range allowed(m.state.View.Items[i]) {
				hints = append(hints, hint{string(op) + " [" + actionKey(op) + "]", 5})
			}
		}
		open := hint{"details [enter]", 2}
		if m.details {
			open = hint{"close [esc]", 7}
		}
		if m.plannable() {
			hints = append(hints, hint{"edit [e]", 3}, hint{"delete [d]", 2})
		}
		hints = append(hints, open, hint{"move [j/k]", 1})
	}
	if m.plannable() {
		hints = append(hints, hint{"new [n]", 4})
	}
	hints = append(hints, hint{"review [r]", 3}, hint{"calendar [c]", 3})
	if m.viewing() != m.today() {
		hints = append(hints, hint{"today [t]", 6})
	}
	hints = append(hints, hint{"help [?]", 8}, hint{"quit [q]", 9})
	for keep := 0; ; keep++ {
		var texts []string
		for _, h := range hints {
			if h.keep >= keep {
				texts = append(texts, h.text)
			}
		}
		row := " " + strings.Join(texts, "  ")
		if ansi.StringWidth(row) <= width || keep > 9 {
			return fit(row, width)
		}
	}
}

// suspended is all that shows in a window smaller than 80 by 24.
func (m *model) suspended() []string {
	if m.width < 4 || m.height < 3 {
		return []string{strings.Repeat(" ", max(m.width, 0))}
	}
	message := []string{"Make the window larger", fmt.Sprintf("flowd needs %d × %d", minWidth, minHeight)}
	rows := []string{m.border("┌", "┐")}
	top := (m.height - 2 - len(message)) / 2
	for i := 0; i < m.height-2; i++ {
		line := ""
		if j := i - top; j >= 0 && j < len(message) {
			text := ansi.Truncate(message[j], m.width-2, "")
			left := (m.width - 2 - ansi.StringWidth(text)) / 2
			line = strings.Repeat(" ", left) + text
		}
		rows = append(rows, m.side(line))
	}
	return append(rows, m.border("└", "┘"))
}

// wrap breaks text into lines of at most width cells, at spaces when it can.
func wrap(text string, width int) []string {
	wrapped := ansi.Wrap(text, width, "")
	return strings.Split(wrapped, "\n")
}

// actionKey is the key shown for an action in the footer.
func actionKey(op planner.Operation) string {
	switch op {
	case planner.Start, planner.Pause, planner.Resume:
		return "space"
	case planner.Reopen:
		return "R"
	}
	for key, action := range trackKeys {
		if action == op {
			return key
		}
	}
	return ""
}

// deleteQuestion asks about the item in one footer row. A long title is
// shortened so the answers always fit.
func (m *model) deleteQuestion(width int) string {
	title := ""
	if i := m.index(); i >= 0 {
		title = m.state.View.Items[i].Title
	}
	answers := "  yes [y]  no [n]"
	room := width - ansi.StringWidth(answers) - len(" Delete \"\"?")
	return fit(" Delete \""+ansi.Truncate(title, max(1, room), "…")+"\"?"+answers, width)
}
