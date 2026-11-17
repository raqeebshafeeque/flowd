package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// calendar is the month view used to pick another day. It only moves a
// cursor; nothing is read until a day is chosen, and choosing a day never
// creates it.
type calendar struct {
	cursor time.Time // midnight UTC of the date under the cursor
}

// calendarHeight is the rows the calendar needs: heading, rule, month,
// weekdays, six weeks and the selected date.
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
	switch msg.String() {
	case "h", "left":
		c.cursor = c.cursor.AddDate(0, 0, -1)
	case "l", "right":
		c.cursor = c.cursor.AddDate(0, 0, 1)
	case "k", "up":
		c.cursor = c.cursor.AddDate(0, 0, -7)
	case "j", "down":
		c.cursor = c.cursor.AddDate(0, 0, 7)
	case "<":
		c.cursor = addMonths(c.cursor, -1)
	case ">":
		c.cursor = addMonths(c.cursor, 1)
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

// addMonths moves by whole months, keeping the day where it can: 31 January
// plus one month is the last day of February, not early March.
func addMonths(t time.Time, months int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(t.Day(), last), 0, 0, 0, 0, time.UTC)
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

// calendarRows draws the month around the cursor.
func (m *model) calendarRows(width int) []string {
	c := m.calendar.cursor
	rows := []string{" " + m.style.paint("heading", "Calendar"), " " + m.style.paint("border", strings.Repeat("─", width-2))}
	rows = append(rows, "  "+c.Format("January 2006"), "   Mo  Tu  We  Th  Fr  Sa  Su")
	first := time.Date(c.Year(), c.Month(), 1, 0, 0, 0, 0, time.UTC)
	offset := (int(first.Weekday()) + 6) % 7 // weeks start on Monday
	days := first.AddDate(0, 1, -1).Day()
	for week := 0; week < 6; week++ {
		var b strings.Builder
		b.WriteString("  ")
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
		rows = append(rows, b.String())
	}
	selected := "  Selected " + c.Format("Mon 2 Jan 2006")
	if c.Format("2006-01-02") == m.today() {
		selected += " (today)"
	}
	return append(rows, selected)
}

func (m *model) calendarFooter() string {
	return " show [enter]  day [h/l]  week [j/k]  month [</>]  today [t]  back [esc]"
}
