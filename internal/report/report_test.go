package report

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

func clock(hh, mm int) time.Time { return time.Date(2026, 11, 19, hh, mm, 0, 0, time.UTC) }

func item(title string, status domain.Status, minutes int64, stretches ...domain.Stretch) planner.ItemInsight {
	var work int64
	for _, s := range stretches {
		if s.Running {
			work += s.To.Sub(s.From).Nanoseconds()
		}
	}
	started := (*time.Time)(nil)
	if len(stretches) > 0 {
		started = &stretches[0].From
	}
	return planner.ItemInsight{
		ItemView:  planner.ItemView{Item: domain.Item{Title: title, Status: status, DurationMinutes: minutes, StartedAt: started}, Metrics: domain.Metrics{WorkNS: work}},
		Stretches: stretches,
	}
}

func run(from, to time.Time) domain.Stretch  { return domain.Stretch{From: from, To: to, Running: true} }
func wait(from, to time.Time) domain.Stretch { return domain.Stretch{From: from, To: to} }

// sampleDay is a worked Thursday: a long item with a pause, a short one, and
// one never started.
func sampleDay() planner.DayInsight {
	d := planner.DayInsight{Date: "2026-11-19", Items: []planner.ItemInsight{
		item("Deep work on the parser", domain.Done, 180, run(clock(9, 0), clock(10, 0)), wait(clock(10, 0), clock(11, 0)), run(clock(11, 0), clock(12, 0))),
		item("Code review", domain.Done, 45, run(clock(10, 0), clock(10, 45))),
		item("Write release notes", domain.Todo, 60),
	}}
	for _, it := range d.Items {
		d.FocusedNS += it.WorkNS
		d.PlannedMinutes += it.DurationMinutes
		d.Total++
		if it.Status == domain.Done {
			d.Finished++
		}
	}
	d.Sessions, d.LongestNS, d.PausedNS = 3, int64(time.Hour), int64(time.Hour)
	d.Breaks = domain.Breaks{Switches: 1, Stops: 0}
	return d
}

func samplePeriod(period planner.Period, from string, days int) planner.PeriodInsight {
	p := planner.PeriodInsight{Period: period, From: from, Today: "2026-11-19"}
	start, _ := time.Parse("2006-01-02", from)
	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		d := planner.DayInsight{Date: date}
		if date <= "2026-11-19" && i%2 == 0 {
			d = sampleDay()
			d.Date = date
			d.FocusedNS = int64(i%7+1) * int64(time.Hour)
		}
		p.Days = append(p.Days, d)
	}
	p.To = p.Days[len(p.Days)-1].Date
	return p
}

func allPages(width int, style Style) map[string][]string {
	p := Page{Width: width, Zone: time.UTC, Style: style}
	return map[string][]string{
		"day":   p.Day(sampleDay()),
		"week":  p.Week(samplePeriod(planner.WeekPeriod, "2026-11-16", 7)),
		"month": p.Month(samplePeriod(planner.MonthPeriod, "2026-11-01", 30)),
	}
}

// No row is ever wider than the page, at any width a terminal might have.
func TestNoRowIsWiderThanThePage(t *testing.T) {
	for width := 60; width <= 160; width += 7 {
		for name, rows := range allPages(width, Style{}) {
			for i, row := range rows {
				if w := ansi.StringWidth(row); w > width-2 {
					t.Fatalf("%s at %d: row %d is %d wide, leaving no margin: %q", name, width, i, w, row)
				}
			}
		}
	}
}

// Color only wraps text. The layout is exactly the same with or without it.
func TestColorNeverMovesAnything(t *testing.T) {
	colored := Style{
		Paint:  func(role, text string) string { return "\x1b[34m" + text + "\x1b[m" },
		Status: func(_ domain.Status, text string) string { return "\x1b[32m" + text + "\x1b[m" },
	}
	plain, painted := allPages(80, Style{}), allPages(80, colored)
	for name := range plain {
		for i := range plain[name] {
			if ansi.Strip(painted[name][i]) != plain[name][i] {
				t.Fatalf("%s row %d: %q became %q", name, i, plain[name][i], ansi.Strip(painted[name][i]))
			}
		}
	}
}

// Every stat value, bar and chart starts in the same column, right after the
// labels.
func TestValuesStartInOneColumn(t *testing.T) {
	rows := allPages(80, Style{})["day"]
	column := len(margin) + labelWidth
	for _, label := range []string{"Focused", "Paused", "Longest", "Completed", "Worked"} {
		found := false
		for _, row := range rows {
			if strings.HasPrefix(row, margin+label+" ") {
				found = true
				if row[column-1] != ' ' || row[column] == ' ' {
					t.Errorf("%s does not start its value at column %d: %q", label, column, row)
				}
			}
		}
		if !found {
			t.Errorf("no %s row", label)
		}
	}
}

func TestBarsShowTheRightShare(t *testing.T) {
	rows := strings.Join(allPages(80, Style{})["day"], "\n")
	for _, want := range []string{"67%   2 of 3 items", "58%   2h45m of 4h45m planned", "Interrupted  1  (1 switch, 0 stops)"} {
		if !strings.Contains(rows, want) {
			t.Errorf("missing %q in:\n%s", want, rows)
		}
	}
	p := Page{Width: 80}
	bar := ansi.Strip(p.bar("Completed", 3, 4, "good", ""))
	if strings.Count(bar, "█") != 18 || strings.Count(bar, "░") != 6 {
		t.Fatalf("three quarters of 24 cells should be filled: %q", bar)
	}
}

// The hours are evenly spaced, and work lands under the hour it happened in.
func TestTheTimelineLinesUpWithItsHours(t *testing.T) {
	rows := allPages(100, Style{})["day"]
	var axisRow, deep string
	for i, row := range rows {
		if strings.TrimSpace(row) == "Timeline" {
			axisRow = rows[i+1]
		}
		if strings.HasPrefix(row, margin+"Deep work") {
			deep = row
		}
	}
	at := func(label string) int { return strings.Index(axisRow, label) }
	if at("09") < 0 || at("10")-at("09") != at("11")-at("10") {
		t.Fatalf("the hours are not evenly spaced: %q", axisRow)
	}
	runes := []rune(deep)
	if runes[at("09")] != '█' || runes[at("10")] != '░' || runes[at("11")] != '█' {
		t.Fatalf("work should start at 09, wait from 10 and run again from 11:\n%s\n%s", axisRow, deep)
	}
}

// Each day's bar sits right under its name, and its numbers under that.
func TestWeekColumnsLineUp(t *testing.T) {
	rows := allPages(80, Style{})["week"]
	var names, focused string
	for _, row := range rows {
		if strings.Contains(row, "Mon    Tue") {
			names = row
		}
		if strings.HasPrefix(row, margin+"Focused ") && strings.Contains(row, "·") {
			focused = row
		}
	}
	for i, day := range []string{"Mon", "Tue", "Wed", "Thu"} {
		centre := strings.Index(names, day) + 1
		columnWidth := Page{Width: 80}.columnWidth()
		start := len(margin) + labelWidth + i*columnWidth
		if centre < start || centre >= start+columnWidth {
			t.Fatalf("%s is not over its column %d: %q", day, start, names)
		}
		if strings.TrimSpace(focused[start:start+columnWidth]) == "" && i%2 == 0 {
			t.Fatalf("%s has no number under it: %q", day, focused)
		}
	}
}

// The heatmap puts each day under its weekday, and shades it by its hours.
func TestTheHeatmapPutsDaysUnderTheirWeekday(t *testing.T) {
	rows := allPages(80, Style{})["month"]
	var header string
	weeks := map[string]string{}
	for _, row := range rows {
		if strings.Contains(row, "Mon    Tue") {
			header = row
		}
		if f := strings.Fields(row); len(f) > 0 && strings.HasPrefix(f[0], "W4") {
			weeks[f[0]] = row
		}
	}
	// 16 November 2026 is a Monday, in week 47.
	week := weeks["W47"]
	if col := strings.Index(week, "16"); col < 0 || col > strings.Index(header, "Mon") || strings.Index(header, "Mon")-col > 2 {
		t.Fatalf("16 should be under Mon:\n%s\n%s", header, week)
	}
	if !strings.Contains(week, "16 ") || strings.Contains(week, "20 ░") {
		t.Fatalf("a future day has no shade: %q", week)
	}
	if shade(0) != "··" || shade(int64(time.Hour)) != "░░" || shade(int64(3*time.Hour)) != "▒▒" || shade(int64(5*time.Hour)) != "▓▓" || shade(int64(7*time.Hour)) != "██" {
		t.Fatal("the shades do not follow the hours")
	}
}

func TestADayWithNothingSaysSo(t *testing.T) {
	p := Page{Width: 80}
	if rows := strings.Join(p.Day(planner.DayInsight{Date: "2026-11-19"}), "\n"); !strings.Contains(rows, "Nothing was planned on this day.") {
		t.Fatalf("got:\n%s", rows)
	}
	idle := planner.DayInsight{Date: "2026-11-19", Items: []planner.ItemInsight{item("Later", domain.Todo, 30)}, Total: 1, PlannedMinutes: 30}
	if rows := strings.Join(p.Day(idle), "\n"); !strings.Contains(rows, "Nothing was started on this day.") || strings.Contains(rows, "working") {
		t.Fatalf("got:\n%s", rows)
	}
}
