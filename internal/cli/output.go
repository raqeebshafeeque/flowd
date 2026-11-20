package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// Report prints an error for a person and returns its exit status. People
// get a sentence and a hint. The code is for scripts and agents, which ask
// for it with --json.
func Report(errOut io.Writer, err error) int {
	f := planner.Describe(err)
	fmt.Fprintln(errOut, "Error: "+sentenceOf(f.Summary))
	if f.Hint != "" {
		fmt.Fprintln(errOut, "       "+sentenceOf(f.Hint))
	}
	return f.Exit
}

// reportJSON prints an error as JSON, with its code, for scripts and agents.
func reportJSON(errOut io.Writer, err error) int {
	f := planner.Describe(err)
	type problem struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint,omitempty"`
	}
	writeJSON(errOut, struct {
		Error problem `json:"error"`
	}{problem{f.Code, f.Summary, f.Hint}})
	return f.Exit
}

// sentenceOf ends text with a full stop unless it already ends a sentence
// or ends in a quoted example.
func sentenceOf(text string) string {
	if text == "" || strings.ContainsAny(text[len(text)-1:], ".!?\"") {
		return text
	}
	return text + "."
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
		return printDated(out, o.look, v.Date, []planner.ItemView{*v.Running})
	case o.command == "review":
		fmt.Fprintf(out, "%s\n\n", heading(v.Date))
		return printReview(out, v)
	}
	if len(v.Items) == 0 {
		_, err := fmt.Fprintf(out, "%s\n\nNothing planned. Add something with: flowd add 1h \"Study\"\n", heading(v.Date))
		return err
	}
	return printDated(out, o.look, v.Date, v.Items)
}

func printReview(out io.Writer, v planner.View) error {
	done, total := v.Finished()
	fmt.Fprintln(out, "Summary")
	fmt.Fprintf(out, "  Total duration   %s\n", planner.TotalDuration(v))
	fmt.Fprintf(out, "  Recorded work    %s\n\n", planner.Span(v.WorkNS))
	fmt.Fprintln(out, "Progress")
	fmt.Fprintf(out, "  %d / %d finished\n\n", done, total)
	fmt.Fprintln(out, "Status")
	for _, status := range domain.Statuses {
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
	if len(r.Carried) > 0 {
		var items []planner.ItemView
		for _, c := range r.Carried {
			items = append(items, planner.ItemView{Item: c})
		}
		if o.json {
			return writeJSON(out, items)
		}
		return printDated(out, o.look, r.Day.Date, items)
	}
	item := planner.ItemView{Item: r.Item, Metrics: r.Metrics}
	if o.json {
		return writeJSON(out, item)
	}
	return printDated(out, o.look, r.Day.Date, []planner.ItemView{item})
}

// heading turns 2026-11-17 into "Tue 17 Nov 2026".
func heading(date string) string {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return d.Format("Mon 2 Jan 2006")
}
