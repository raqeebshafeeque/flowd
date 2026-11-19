package cli

import (
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

const reset = "\x1b[m"

// statusColors match the workspace, so a status reads the same everywhere.
var statusColors = map[domain.Status]string{
	domain.InProgress: "\x1b[34m", // blue: being worked on
	domain.Paused:     "\x1b[33m", // yellow: waiting
	domain.Done:       "\x1b[32m", // green: finished
	domain.Skipped:    "\x1b[2m",
	domain.Incomplete: "\x1b[2;31m",
}

// look says how output is drawn: in color or not, and how wide the
// terminal is. A width of zero means unknown, and nothing is shortened.
type look struct {
	color bool
	width int
}

func (l look) paint(code, text string) string {
	if !l.color || code == "" {
		return text
	}
	return code + text + reset
}

// minTitle is the narrowest a title column gets before the table is
// allowed to run past the edge of the terminal.
const minTitle = 12

// printTable draws items in a box like the workspace, with the ID, the
// title, the work and duration, and the status. A title that does not fit
// the terminal is shortened with an ellipsis.
func printTable(out io.Writer, l look, items []planner.ItemView) error {
	headers := [4]string{"ID", "TITLE", "DURATION", "STATUS"}
	rows := make([][4]string, len(items))
	for i, item := range items {
		rows[i] = [4]string{string(item.ID), item.Title, planner.Effort(item), statusLabel(item)}
	}
	var widths [4]int
	for c, h := range headers {
		widths[c] = ansi.StringWidth(h)
	}
	for _, row := range rows {
		for c, cell := range row {
			widths[c] = max(widths[c], ansi.StringWidth(cell))
		}
	}
	// Each cell has a space on both sides and a line after it, plus the
	// line on the far left.
	total := 1
	for _, w := range widths {
		total += w + 3
	}
	if l.width > 0 && total > l.width {
		widths[1] = max(minTitle, widths[1]-(total-l.width))
	}
	line := func(left, middle, right string) string {
		parts := make([]string, len(widths))
		for c, w := range widths {
			parts[c] = strings.Repeat("─", w+2)
		}
		return left + strings.Join(parts, middle) + right + "\n"
	}
	// cell pads text to its column. ID and title sit on the left, the
	// numbers and the status on the right, like in the workspace.
	cell := func(c int, text, color string) string {
		text = ansi.Truncate(text, widths[c], "…")
		pad := strings.Repeat(" ", widths[c]-ansi.StringWidth(text))
		if c >= 2 {
			return " " + pad + l.paint(color, text) + " │"
		}
		return " " + l.paint(color, text) + pad + " │"
	}
	var b strings.Builder
	b.WriteString(line("┌", "┬", "┐"))
	b.WriteString("│")
	for c, h := range headers {
		b.WriteString(cell(c, h, ""))
	}
	b.WriteString("\n")
	b.WriteString(line("├", "┼", "┤"))
	for i, row := range rows {
		b.WriteString("│")
		for c, text := range row {
			color := ""
			if c == 3 && !items[i].Archived {
				color = statusColors[items[i].Status]
			}
			b.WriteString(cell(c, text, color))
		}
		b.WriteString("\n")
	}
	b.WriteString(line("└", "┴", "┘"))
	_, err := io.WriteString(out, b.String())
	return err
}

// statusLabel is the status in capitals, or DELETED for a deleted item.
func statusLabel(item planner.ItemView) string {
	if item.Archived {
		return "DELETED"
	}
	return planner.Label(item.Status)
}

// printDated shows the day the items belong to, then the items, so every
// table says which day it is about, including one picked with --date.
func printDated(out io.Writer, l look, date string, items []planner.ItemView) error {
	if _, err := io.WriteString(out, heading(date)+"\n"); err != nil {
		return err
	}
	return printTable(out, l, items)
}
