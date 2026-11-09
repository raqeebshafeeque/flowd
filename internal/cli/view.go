package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

type dayView struct {
	Date     string         `json:"date"`
	Timezone string         `json:"timezone"`
	Blocks   []domain.Block `json:"blocks"`
	Now      *domain.Block  `json:"now"`
	Next     *domain.Block  `json:"next"`
	domain.Review
}

func makeView(day domain.Day, blocks []domain.Block, now time.Time) (dayView, error) {
	view := dayView{Date: day.Date, Timezone: day.Timezone, Blocks: blocks, Now: domain.Now(blocks, now), Next: domain.Next(blocks, now)}
	var err error
	view.Review, err = domain.ReviewDay(day, blocks, now)
	return view, err
}

func overrun(block domain.Block, now time.Time) bool { return domain.IsOverrun(block, now) }

func printView(out io.Writer, opts options, view dayView, now time.Time, zone *time.Location) error {
	if opts.command == "now" || opts.command == "next" {
		block := view.Now
		if opts.command == "next" {
			block = view.Next
		}
		if opts.json {
			if block == nil {
				return json.NewEncoder(out).Encode(nil)
			}
			return json.NewEncoder(out).Encode(struct {
				*domain.Block
				Overrun bool `json:"overrun"`
			}{block, overrun(*block, now)})
		}
		text := describe(block, zone)
		if block != nil && overrun(*block, now) {
			text += " [overrun]"
		}
		_, err := fmt.Fprintf(out, "%s: %s\n", opts.command, text)
		return err
	}
	if opts.json {
		return json.NewEncoder(out).Encode(view)
	}
	progress := "—"
	if view.Progress != nil {
		progress = fmt.Sprintf("%d%%", *view.Progress)
	}
	if _, err := fmt.Fprintf(out, "%s  %s\nprogress: %s\nnow: %s\nnext: %s\n", view.Date, view.Timezone, progress, describe(view.Now, zone), describe(view.Next, zone)); err != nil {
		return err
	}
	if opts.command == "review" {
		if _, err := fmt.Fprintf(out, "done: %d  skipped: %d  unfinished: %d  missed: %d  overruns: %d\n", view.Done, view.Skipped, view.Unfinished, len(view.Missed), len(view.Overruns)); err != nil {
			return err
		}
	}
	if len(view.Blocks) == 0 {
		_, err := fmt.Fprintln(out, "no blocks planned")
		return err
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "#\tplanned\tstatus\ttitle\tid\tflags")
	for index, block := range view.Blocks {
		flag := ""
		for _, id := range view.Missed {
			if block.ID == id {
				flag = "missed"
			}
		}
		if overrun(block, now) {
			flag = "overrun"
		}
		fmt.Fprintf(table, "%d\t%s\t%s\t%s\t%s\t%s\n", index+1, plannedRange(block, zone), clean(string(block.Status)), clean(block.Title), clean(string(block.ID)), flag)
	}
	return table.Flush()
}

func plannedRange(block domain.Block, zone *time.Location) string {
	start, end := block.PlannedStart.In(zone), block.PlannedEnd.In(zone)
	last := end.Format("15:04")
	if start.Format("2006-01-02") != end.Format("2006-01-02") {
		last = end.Format("01-02 15:04")
	}
	return start.Format("15:04") + "-" + last
}

func describe(block *domain.Block, zone *time.Location) string {
	if block == nil {
		return "none"
	}
	return fmt.Sprintf("%s %s [%s] (%s)", plannedRange(*block, zone), clean(block.Title), clean(string(block.Status)), clean(string(block.ID)))
}

func clean(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) || unicode.In(character, unicode.Zl, unicode.Zp) {
			return ' '
		}
		return character
	}, value)
}
