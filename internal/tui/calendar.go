package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// calendar is the month view used to pick another day. It only moves a
// cursor; nothing is read until a day is chosen, and choosing a day never
// creates it.
type calendar struct {
	cursor time.Time // midnight UTC of the date under the cursor

	// input is the month or year being typed after m or y, shown in the
	// footer while the month stays visible.
	input      *field
	prompt     string // "month" or "year"
	inputError string
}

// calendarHeight is the rows the calendar needs: heading, rule, month, a
// gap, weekdays and six weeks.
const calendarHeight = 11

// today is the current date in this computer's timezone.
func (m *model) today() string { return m.now().In(m.zone).Format("2006-01-02") }

// viewing is the date on screen.
func (m *model) viewing() string {
	if m.date != "" {
		return m.date
	}
	return m.today()
}

func (m *model) openCalendar() {
	if m.busy() {
		return
	}
	start, err := time.Parse("2006-01-02", m.viewing())
	if err != nil {
		return
	}
	m.calendar, m.details, m.review = &calendar{cursor: start}, false, false
}

// calendarKey handles keys while the calendar is open.
func (m *model) calendarKey(msg tea.KeyPressMsg) tea.Cmd {
	c := m.calendar
	if c.input != nil {
		c.inputKey(msg)
		return nil
	}
	switch msg.String() {
	case "h", "left":
		c.cursor = c.cursor.AddDate(0, 0, -1)
	case "l", "right":
		c.cursor = c.cursor.AddDate(0, 0, 1)
	case "k", "up":
		c.cursor = c.cursor.AddDate(0, 0, -7)
	case "j", "down":
		c.cursor = c.cursor.AddDate(0, 0, 7)
	case "m":
		f := newField(strconv.Itoa(int(c.cursor.Month())))
		c.input, c.prompt = &f, "month"
	case "y":
		f := newField(strconv.Itoa(c.cursor.Year()))
		c.input, c.prompt = &f, "year"
	case "t":
		c.cursor, _ = time.Parse("2006-01-02", m.today())
	case "enter":
		m.calendar = nil
		return m.show(c.cursor.Format("2006-01-02"))
	case "esc":
		m.calendar = nil
	}
	return nil
}

// inputKey edits the typed month or year. Enter jumps to it, keeping the
// day where it can. A number out of range stays with an explanation and
// nothing moves. Esc goes back without moving.
func (c *calendar) inputKey(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc":
		c.input, c.inputError = nil, ""
	case "enter":
		value, err := strconv.Atoi(strings.TrimSpace(c.input.value()))
		year, month := c.cursor.Year(), c.cursor.Month()
		switch {
		case c.prompt == "month" && (err != nil || value < 1 || value > 12):
			c.inputError = "Enter a month from 1 to 12"
			return
		case c.prompt == "year" && (err != nil || value < 1 || value > 9999):
			c.inputError = "Enter a year from 1 to 9999"
			return
		case c.prompt == "month":
			month = time.Month(value)
		default:
			year = value
		}
		first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		last := first.AddDate(0, 1, -1).Day()
		c.cursor = time.Date(year, month, min(c.cursor.Day(), last), 0, 0, 0, 0, time.UTC)
		c.input, c.inputError = nil, ""
	default:
		if !c.input.edit(msg.String()) && msg.Text != "" {
			c.input.insert(msg.Text)
		}
	}
}

// show switches the workspace to a date. Today is remembered as "today", so
// the workspace moves on by itself at midnight.
func (m *model) show(date string) tea.Cmd {
	if date == m.today() {
		date = ""
	}
	if date == m.date && m.loaded {
		return nil
	}
	m.date, m.loaded, m.selected, m.details, m.err = date, false, "", false, nil
	return m.read()
}

// calendarRows draws the month around the cursor. The month name, weekdays
// and dates all line up under the Calendar heading, and the cursor date is
// in brackets and teal.
func (m *model) calendarRows(width int) []string {
	c := m.calendar.cursor
	rows := []string{" " + m.style.paint("heading", "Calendar"), " " + m.style.paint("border", strings.Repeat("─", width-2))}
	rows = append(rows, " "+c.Format("January 2006"), "", " Mo  Tu  We  Th  Fr  Sa  Su")
	first := time.Date(c.Year(), c.Month(), 1, 0, 0, 0, 0, time.UTC)
	offset := (int(first.Weekday()) + 6) % 7 // weeks start on Monday
	days := first.AddDate(0, 1, -1).Day()
	for week := 0; week < 6; week++ {
		var b strings.Builder
		for weekday := 0; weekday < 7; weekday++ {
			day := week*7 + weekday - offset + 1
			switch {
			case day < 1 || day > days:
				b.WriteString("    ")
			case day == c.Day():
				b.WriteString(m.style.paint("selected", fmt.Sprintf("[%2d]", day)))
			default:
				fmt.Fprintf(&b, " %2d ", day)
			}
		}
		rows = append(rows, strings.TrimRight(b.String(), " "))
	}
	return rows
}

// calendarFooter shows the calendar keys, or the month or year being typed.
func (m *model) calendarFooter() string {
	c := m.calendar
	if c.input == nil {
		return " show [enter]  move [h/j/k/l]  month [m]  year [y]  today [t]  back [esc]"
	}
	row := " " + m.calendarPrompt() + c.input.value()
	if c.inputError != "" {
		row += "  " + m.style.paint("error", c.inputError)
	}
	return row + "  " + m.style.paint("muted", "go [enter]  back [esc]")
}

func (m *model) calendarPrompt() string {
	if m.calendar.prompt == "year" {
		return "Year: "
	}
	return "Month (1-12): "
}

// calendarCaret is where the terminal cursor goes while typing a month or
// year: in the footer, after the typed text so far.
func (m *model) calendarCaret() *tea.Cursor {
	c := m.calendar
	if c == nil || c.input == nil {
		return nil
	}
	_, col := c.input.window(20)
	cursor := tea.NewCursor(2+len(m.calendarPrompt())+col, m.height-2)
	cursor.Shape = tea.CursorBar
	return cursor
}
