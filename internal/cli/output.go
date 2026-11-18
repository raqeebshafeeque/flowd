package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// Report prints an error to stderr and returns its exit status.
func Report(errOut io.Writer, err error) int {
	f := planner.Describe(err)
	fmt.Fprintln(errOut, f.Code)
	fmt.Fprintln(errOut, f.Summary)
	if f.Hint != "" {
		fmt.Fprintln(errOut, f.Hint)
	}
	return f.Exit
}

func writeJSON(out io.Writer, value any) error {
	return json.NewEncoder(out).Encode(value)
}

// printDay prints list, review or now.
func printDay(out io.Writer, o options, v planner.View) error {
	switch {
	case o.command == "now" && o.json:
		return writeJSON(out, v.Running)
	case o.json:
		return writeJSON(out, v)
	case o.command == "now":
		if v.Running == nil {
			_, err := fmt.Fprintln(out, "Nothing is running")
			return err
		}
		_, err := fmt.Fprintf(out, "%s, %s of work\n", v.Running.Title, planner.Span(v.Running.WorkNS))
		return err
	}
	fmt.Fprintf(out, "%s · %s\n\n", heading(v.Date), v.Timezone)
	if o.command == "review" {
		return printReview(out, v)
	}
	if len(v.Items) == 0 {
		_, err := fmt.Fprintln(out, "Nothing planned. Add something with: flowd add 1h \"Study\"")
		return err
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "STATUS\tWORK / DURATION\tTITLE\tID")
	for _, item := range v.Items {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", planner.Label(item.Status), planner.Effort(item), item.Title, item.ID)
	}
	return table.Flush()
}

func printReview(out io.Writer, v planner.View) error {
	done, total := v.Finished()
	fmt.Fprintln(out, "Summary")
	fmt.Fprintf(out, "  Total duration   %s\n", planner.TotalDuration(v))
	fmt.Fprintf(out, "  Recorded work    %s\n\n", planner.Span(v.WorkNS))
	fmt.Fprintln(out, "Progress")
	fmt.Fprintf(out, "  %d / %d finished\n\n", done, total)
	fmt.Fprintln(out, "Status")
	for _, status := range []domain.Status{domain.Todo, domain.InProgress, domain.Paused, domain.Done, domain.Skipped, domain.Incomplete} {
		fmt.Fprintf(out, "  %s (%d)\n", planner.Label(status), v.Counts[status])
		for _, item := range v.Items {
			if item.Status == status {
				fmt.Fprintf(out, "  • %s\n", item.Title)
			}
		}
	}
	return nil
}

// printResult prints a confirmed change.
func printResult(out io.Writer, o options, r planner.Result) error {
	item := planner.ItemView{Item: r.Item, Metrics: r.Metrics}
	if o.json {
		return writeJSON(out, item)
	}
	verb := map[string]string{"add": "Planned", "edit": "Updated", "delete": "Deleted", "start": "Started", "pause": "Paused", "resume": "Resumed", "finish": "Finished", "skip": "Skipped", "unskip": "Brought back", "reopen": "Reopened"}[o.command]
	fmt.Fprintf(out, "%s %s (%s, %s) %s\n", verb, item.Title, planner.Label(item.Status), planner.Effort(item), item.ID)
	for _, id := range r.AutoPaused {
		fmt.Fprintf(out, "Paused %s\n", titleOf(r, id))
	}
	return nil
}

func titleOf(r planner.Result, id domain.ID) string {
	if events := r.History[id]; len(events) > 0 {
		return events[len(events)-1].After.Title
	}
	return string(id)
}

func printEvents(out io.Writer, o options, events []domain.Event) error {
	if o.json {
		return writeJSON(out, events)
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "SEQ\tWHEN (UTC)\tCHANGE\tREASON")
	for _, e := range events {
		change := "created as todo"
		if e.Before != nil {
			change = fmt.Sprintf("%s to %s", planner.Words(e.FromStatus), planner.Words(e.ToStatus))
			if e.FromStatus == e.ToStatus {
				change = "edited"
				if e.After.Archived {
					change = "deleted"
				}
			}
		}
		fmt.Fprintf(table, "%d\t%s\t%s\t%s\n", e.Seq, e.OccurredAt.UTC().Format("2006-01-02 15:04:05"), change, e.Reason)
	}
	return table.Flush()
}

// heading turns 2026-11-17 into "Tue 17 Nov 2026".
func heading(date string) string {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return d.Format("Mon 2 Jan 2006")
}
