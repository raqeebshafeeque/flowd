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
	footer := m.footer(inner)
	body := m.height - 5 - len(footer)
	rows := []string{m.headerEdge("┌", "┬", "┐"), m.side(m.header(inner)), m.headerEdge("├", "┴", "┤")}
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
	} else if m.carrying != nil {
		panel := m.carryHeight(body)
		content = append(m.list(inner, body-panel-1), m.style.paint("border", strings.Repeat("─", inner)))
		content = append(content, pad(m.carryRows(inner, panel), panel)...)
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
	rows = append(rows, m.border("├", "┤"))
	for _, row := range footer {
		rows = append(rows, m.side(row))
	}
	return append(rows, m.border("└", "┘"))
}

// detailsHeight is about a quarter of the body, never less than ten rows:
// the heading, its rule, the seven fields and a correction when there is
// one.
func detailsHeight(body int) int { return min(max(10, body/4+1), body-3) }

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
	cells := headerCells(width)
	bar := m.style.paint("border", "│")
	return " " + m.style.paint("brand", "flowd") + " " + bar +
		fit(" "+m.headerNote(now), cells[1]) + bar +
		" " + now.Format("Mon 02 Jan 2006") + " " + bar +
		" " + now.Format("15:04:05") + " "
}

// headerCells are the widths of the header's four cells: the brand, a note
// about the day on screen, the date and the clock.
func headerCells(width int) [4]int { return [4]int{7, max(0, width-37), 17, 10} }

// headerNote says which day is on screen when it is not today. Today leaves
// the middle of the header empty.
func (m *model) headerNote(now time.Time) string {
	day := m.viewing()
	switch {
	case !m.loaded && m.err == nil:
		return "Loading: " + day
	case m.ordering != nil:
		return "Reordering"
	case day != now.Format("2006-01-02"):
		return "Viewing: " + day
	}
	return ""
}

// headerEdge draws the border above or below the header, with joints where
// the header's dividers meet it.
func (m *model) headerEdge(left, joint, right string) string {
	cells := headerCells(m.width - 2)
	parts := make([]string, len(cells))
	for i, w := range cells {
		parts[i] = strings.Repeat("─", w)
	}
	return m.style.paint("border", left+strings.Join(parts, joint)+right)
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
		{"Paused", planner.Paused(item)},
		{"Started At", m.instant(item.StartedAt)},
		{"Finished At", m.instant(item.FinishedAt)},
	}
	if item.CorrectedAt != nil {
		fields = append(fields, [2]string{"Corrected", "Marked done later, on " + item.CorrectedAt.In(m.zone).Format("Mon 2 Jan")})
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
func (m *model) footer(width int) []string {
	one := func(row string) []string { return []string{fit(row, width)} }
	switch {
	case m.form != nil:
		return one(m.formFooter())
	case m.deleting != "":
		return []string{m.deleteQuestion(width)}
	case m.correcting != "":
		return []string{m.correctQuestion(width)}
	case m.help:
		return one(" scroll [j/k]  back [esc]")
	case m.carrying != nil:
		return one(m.carryFooter())
	case m.ordering != nil:
		return one(m.orderFooter())
	case m.calendar != nil:
		return one(m.calendarFooter())
	case m.review:
		return one(" scroll [j/k]  back [esc]  quit [q]")
	case m.pending:
		return one(" Saving…")
	}
	return m.hintRows(m.hints(), width)
}

// hints are the keys the footer offers, the ones for the selected item and
// the day first and help and quit last.
func (m *model) hints() []string {
	var hints []string
	if i := m.index(); i >= 0 {
		if m.trackable() {
			item := m.state.View.Items[i]
			for _, op := range allowed(item) {
				hints = append(hints, actionName(op, item.Status)+" ["+actionKey(op)+"]")
			}
		}
		if m.correctable() {
			hints = append(hints, "mark done [f]")
		}
		if m.plannable() {
			hints = append(hints, "edit [e]", "delete [d]")
		}
		if m.details {
			hints = append(hints, "close [esc]")
		} else {
			hints = append(hints, "details [enter]")
		}
		hints = append(hints, "move [j/k]")
	}
	if m.plannable() {
		hints = append(hints, "new [n]")
		if len(m.state.View.Items) > 1 {
			hints = append(hints, "order [o]")
		}
		hints = append(hints, "carry [C]")
	}
	hints = append(hints, "review [r]", "calendar [c]")
	if m.viewing() != m.today() {
		hints = append(hints, "today [t]")
	}
	return append(hints, "help [?]", "quit [q]")
}

const (
	moreHint = "more [.]"
	lessHint = "less [.]"
)

// hintRows lays the hints out on one row. When they do not all fit, the ones
// that fit stay and more [.] sits at the right edge. Pressing . opens the
// rest on the rows below, with less [.] in its place, and any other key
// folds them away again. No hint is ever dropped.
func (m *model) hintRows(hints []string, width int) []string {
	row := func(items []string, edge string) string {
		left := " " + strings.Join(items, "  ")
		if edge == "" {
			return fit(left, width)
		}
		return fit(left+strings.Repeat(" ", max(2, width-ansi.StringWidth(left)-ansi.StringWidth(edge)-1))+edge+" ", width)
	}
	if ansi.StringWidth(" "+strings.Join(hints, "  ")) <= width {
		m.moreHints = false
		return []string{row(hints, "")}
	}
	var shown, hidden []string
	used := 1 + ansi.StringWidth(moreHint) + 3
	for i, h := range hints {
		need := ansi.StringWidth(h) + 2
		if len(hidden) == 0 && used+need <= width {
			shown = append(shown, h)
			used += need
			continue
		}
		hidden = append(hidden, hints[i:]...)
		break
	}
	if !m.moreHints {
		return []string{row(shown, moreHint)}
	}
	rows := []string{row(shown, lessHint)}
	var line []string
	for _, h := range hidden {
		if len(line) > 0 && ansi.StringWidth(" "+strings.Join(append(line, h), "  ")) > width {
			rows = append(rows, row(line, ""))
			line = nil
		}
		line = append(line, h)
	}
	return append(rows, row(line, ""))
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
	if op == planner.Finish {
		return "f"
	}
	return "space"
}

// actionName is what the footer calls an action: starting a paused item
// resumes it, and starting a done one reopens it.
func actionName(op planner.Operation, status domain.Status) string {
	switch {
	case op == planner.Start && status == domain.Paused:
		return "resume"
	case op == planner.Start && status == domain.Done:
		return "reopen"
	}
	return string(op)
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
