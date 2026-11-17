package tui

import (
	"strings"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// noticeTime is how long an error stays on screen.
const noticeTime = 7 * time.Second

// notice is an error shown as a small popup above the footer.
type notice struct {
	feedback planner.Feedback
	until    time.Time
}

// popup shows err for seven seconds. A new error replaces the old one.
func (m *model) popup(err error) {
	f := planner.Describe(err)
	m.notice, m.lastError = &notice{feedback: f, until: m.now().Add(noticeTime)}, &f
}

// visibleNotice returns the notice if it has not expired yet.
func (m *model) visibleNotice() *notice {
	if m.notice == nil || !m.now().Before(m.notice.until) {
		return nil
	}
	return m.notice
}

// noticeRows draws the popup: a box with the summary and the hint, as wide
// as the body.
func (m *model) noticeRows(width int) []string {
	n := m.visibleNotice()
	if n == nil {
		return nil
	}
	inner := width - 6
	lines := []string{m.style.paint("error", n.feedback.Summary)}
	for _, line := range wrap(n.feedback.Hint, inner) {
		if line != "" {
			lines = append(lines, m.style.paint("muted", line))
		}
	}
	edge := func(left, right string) string {
		return " " + m.style.paint("error", left+strings.Repeat("─", width-4)+right)
	}
	rows := []string{edge("┌", "┐")}
	for _, line := range lines {
		rows = append(rows, " "+m.style.paint("error", "│")+" "+fit(line, inner)+" "+m.style.paint("error", "│"))
	}
	return append(rows, edge("└", "┘"))
}
