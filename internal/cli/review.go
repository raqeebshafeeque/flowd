package cli

import (
	"context"
	"io"
	"strings"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
	"github.com/raqeebshafeeque/flowd/internal/report"
)

// roles color a review at a terminal, matching the workspace.
var roles = map[string]string{
	"heading": "\x1b[1m",
	"label":   "\x1b[2m",
	"muted":   "\x1b[2m",
	"work":    "\x1b[34m",
	"paused":  "\x1b[33m",
	"good":    "\x1b[32m",
	"today":   "\x1b[1;36m",
}

// review prints how the day, its week or its month went. The day's JSON is
// the same view flowd list prints, and a week's or a month's is the whole
// period with every day in it.
func review(ctx context.Context, out io.Writer, o options, client planner.Client, env Env) error {
	period := planner.DayPeriod
	switch {
	case o.week:
		period = planner.WeekPeriod
	case o.month:
		period = planner.MonthPeriod
	}
	if o.json && period == planner.DayPeriod {
		state, err := client.Read(ctx, planner.Selection{Date: o.date, Timezone: o.zone})
		if err != nil {
			return err
		}
		return writeJSON(out, state.View)
	}
	insight, err := client.Review(ctx, period, o.date)
	if err != nil {
		return err
	}
	if o.json {
		return writeJSON(out, insight)
	}
	page := report.Page{Width: o.look.width, Zone: env.Zone, Style: reviewStyle(o.look)}
	if page.Width <= 0 {
		page.Width = 80
	}
	var rows []string
	switch period {
	case planner.WeekPeriod:
		rows = page.Week(insight)
	case planner.MonthPeriod:
		rows = page.Month(insight)
	default:
		rows = page.Day(insight.Days[0])
	}
	_, err = io.WriteString(out, strings.Join(rows, "\n")+"\n")
	return err
}

func reviewStyle(l look) report.Style {
	if !l.color {
		return report.Style{}
	}
	return report.Style{
		Paint:  func(role, text string) string { return l.paint(roles[role], text) },
		Status: func(s domain.Status, text string) string { return l.paint(statusColors[s], text) },
	}
}
