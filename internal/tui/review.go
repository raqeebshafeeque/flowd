package tui

import (
	"fmt"
	"strings"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// The review is a read-only report of the day on screen: totals, progress
// and every item grouped by status. It reads exactly what `flowd review`
// prints, and j and k scroll it.

var reviewStatuses = domain.Statuses

func (m *model) openReview() {
	if m.loaded && !m.busy() {
		m.review, m.reviewOffset, m.details = true, 0, false
	}
}

// reviewKey handles keys while the review is open.
func (m *model) reviewKey(key string) {
	switch key {
	case "j", "down":
		m.reviewOffset++
	case "k", "up":
		m.reviewOffset = max(0, m.reviewOffset-1)
	case "esc", "r":
		m.review = false
	}
}

// reviewRows draws the whole report, before scrolling.
func (m *model) reviewRows(width int) []string {
	v := m.state.View
	// One empty row above the first section and two between sections, so
	// each section reads as its own block.
	section := func(title string) []string {
		return []string{"", "", " " + m.style.paint("heading", title), " " + m.style.paint("border", strings.Repeat("─", width-2))}
	}
	rows := section("Summary")[1:]
	rows = append(rows,
		" Total duration   "+planner.TotalDuration(v),
		" Recorded work    "+planner.Span(v.WorkNS))
	done, total := v.Finished()
	rows = append(rows, section("Progress")...)
	rows = append(rows, fmt.Sprintf(" %d / %d finished", done, total))
	rows = append(rows, section("Status")...)
	for i, status := range reviewStatuses {
		if i > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, " "+m.style.status(status, fmt.Sprintf("%s (%d)", planner.Label(status), v.Counts[status])))
		for _, item := range v.Items {
			if item.Status != status {
				continue
			}
			for j, line := range wrap(item.Title, width-5) {
				prefix := "   "
				if j == 0 {
					prefix = " • "
				}
				rows = append(rows, prefix+line)
			}
		}
	}
	return rows
}

// reviewBody is the part of the report that fits, after scrolling. The
// offset is kept inside the report so scrolling past an end does nothing.
func (m *model) reviewBody(width, height int) []string {
	rows := m.reviewRows(width)
	m.reviewOffset = min(max(m.reviewOffset, 0), max(0, len(rows)-height))
	return pad(rows[m.reviewOffset:], height)
}
