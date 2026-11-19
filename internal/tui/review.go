package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
	"github.com/raqeebshafeeque/flowd/internal/report"
)

// The review is a read-only report of how a day, its week or its month
// went, drawn exactly like flowd review. Tab or 1, 2 and 3 switch between
// them, h and l go to the previous or next one, t comes back to the current
// one and j and k scroll.

// reviewMsg is a review that has been read.
type reviewMsg struct {
	insight planner.PeriodInsight
	err     error
	period  planner.Period
	anchor  string
}

var periods = []planner.Period{planner.DayPeriod, planner.WeekPeriod, planner.MonthPeriod}

func (m *model) openReview() tea.Cmd {
	if !m.loaded || m.busy() {
		return nil
	}
	m.review, m.reviewOffset, m.details = true, 0, false
	m.reviewPeriod, m.reviewAnchor, m.reviewData = planner.DayPeriod, m.state.View.Date, nil
	return m.readReview()
}

// readReview loads the period on screen in the background.
func (m *model) readReview() tea.Cmd {
	period, anchor := m.reviewPeriod, m.reviewAnchor
	return func() tea.Msg {
		insight, err := m.client.Review(m.ctx, period, anchor)
		return reviewMsg{insight, err, period, anchor}
	}
}

// receiveReview keeps a review only if it is still the one on screen.
func (m *model) receiveReview(msg reviewMsg) {
	if !m.review || msg.period != m.reviewPeriod || msg.anchor != m.reviewAnchor {
		return
	}
	m.reviewErr = msg.err
	if msg.err == nil {
		m.reviewData = &msg.insight
	}
}

// reviewKey handles keys while the review is open.
func (m *model) reviewKey(key string) tea.Cmd {
	switch key {
	case "j", "down":
		m.reviewOffset++
		return nil
	case "k", "up":
		m.reviewOffset = max(0, m.reviewOffset-1)
		return nil
	case "esc", "r":
		m.review = false
		return nil
	case "tab":
		for i, p := range periods {
			if p == m.reviewPeriod {
				return m.showPeriod(periods[(i+1)%len(periods)], m.reviewAnchor)
			}
		}
	case "1", "2", "3":
		return m.showPeriod(periods[key[0]-'1'], m.reviewAnchor)
	case "h", "left":
		return m.showPeriod(m.reviewPeriod, shift(m.reviewPeriod, m.reviewAnchor, -1))
	case "l", "right":
		return m.showPeriod(m.reviewPeriod, shift(m.reviewPeriod, m.reviewAnchor, 1))
	case "t":
		return m.showPeriod(m.reviewPeriod, m.today())
	}
	return nil
}

func (m *model) showPeriod(period planner.Period, anchor string) tea.Cmd {
	if period == m.reviewPeriod && anchor == m.reviewAnchor {
		return nil
	}
	m.reviewPeriod, m.reviewAnchor, m.reviewOffset, m.reviewData, m.reviewErr = period, anchor, 0, nil, nil
	return m.readReview()
}

// shift moves a date by one day, week or month. A month keeps the first of
// the month, so the 31st never skips a short month.
func shift(period planner.Period, date string, by int) string {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	switch period {
	case planner.WeekPeriod:
		d = d.AddDate(0, 0, 7*by)
	case planner.MonthPeriod:
		d = time.Date(d.Year(), d.Month()+time.Month(by), 1, 0, 0, 0, 0, time.UTC)
	default:
		d = d.AddDate(0, 0, by)
	}
	return d.Format("2006-01-02")
}

// reviewRows draws the whole review, before scrolling: the period tabs and
// then the report.
func (m *model) reviewRows(width int) []string {
	// The tabs start where the report does. The one on screen is in
	// brackets and teal, like the cursor date in the calendar.
	tabs := " "
	for _, p := range periods {
		name := map[planner.Period]string{planner.DayPeriod: "Day", planner.WeekPeriod: "Week", planner.MonthPeriod: "Month"}[p]
		if p == m.reviewPeriod {
			tabs += m.style.paint("selected", "["+name+"]") + "  "
		} else {
			tabs += m.style.paint("muted", " "+name+" ") + "  "
		}
	}
	rows := []string{tabs, ""}
	switch {
	case m.reviewErr != nil:
		return append(rows, " "+m.style.paint("error", planner.Describe(m.reviewErr).Summary))
	case m.reviewData == nil:
		return append(rows, " "+m.style.paint("muted", "Reading…"))
	}
	page := report.Page{Width: width, Zone: m.zone, Style: report.Style{
		Paint:  m.style.paint,
		Status: func(s domain.Status, text string) string { return m.style.status(s, text) },
	}}
	switch m.reviewPeriod {
	case planner.WeekPeriod:
		return append(rows, page.Week(*m.reviewData)...)
	case planner.MonthPeriod:
		return append(rows, page.Month(*m.reviewData)...)
	}
	return append(rows, page.Day(m.reviewData.Days[0])...)
}

// reviewBody is the part of the review that fits, after scrolling. The
// offset is kept inside the review so scrolling past an end does nothing.
func (m *model) reviewBody(width, height int) []string {
	rows := m.reviewRows(width)
	m.reviewOffset = min(max(m.reviewOffset, 0), max(0, len(rows)-height))
	return pad(rows[m.reviewOffset:], height)
}
