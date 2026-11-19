package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// statusLine prints the running item as one short line for a status bar
// or a prompt, like "Deep work 1h03m/2h", and "+10m" once it runs past
// its duration. It prints nothing when nothing runs, and also when the
// database is missing, busy or broken, because a prompt is the wrong place
// for an error. flowd doctor explains those.
func statusLine(ctx context.Context, out io.Writer, o options, client planner.Client) error {
	item, ok, err := client.Running(ctx)
	if err != nil || !ok {
		return nil
	}
	_, err = fmt.Fprintln(out, shortLine(item, o.max))
	return err
}

// shortLine writes an item as its title, cut to max columns, then its work
// and duration, and how far it ran over, if it did. Work is in whole
// minutes, so a bar that refreshes every second does not flicker.
func shortLine(item planner.ItemView, max int) string {
	work := item.WorkNS / int64(time.Minute)
	duration, _ := domain.FormatDuration(item.DurationMinutes)
	line := ansi.Truncate(item.Title, max, "…") + " " + planner.Span(work*int64(time.Minute)) + "/" + duration
	if over := work - item.DurationMinutes; over > 0 {
		line += " +" + planner.Span(over*int64(time.Minute))
	}
	return line
}
