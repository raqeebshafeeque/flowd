package report

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// Style colors parts of a review. Paint gets a role, one of heading,
// label, muted, work, paused, good and today. Status colors a status the
// way the rest of flowd does. Either may be nil for plain text.
type Style struct {
	Paint  func(role, text string) string
	Status func(status domain.Status, text string) string
}

// Page draws reviews Width columns wide, with times in Zone.
type Page struct {
	Width int
	Zone  *time.Location
	Style Style
}

// margin starts every row, and the same room is kept free at the end, so
// the review never touches either edge.
const margin = "  "

// room is how many columns a row may use between the two margins.
func (p Page) room() int { return p.Width - 2*width(margin) }

// labelWidth is the width of the labels on the left, the same in every
// section, so stats, bars and charts all start in the same column.
const labelWidth = 13

func (p Page) paint(role, text string) string {
	if p.Style.Paint == nil || text == "" {
		return text
	}
	return p.Style.Paint(role, text)
}

func (p Page) status(s domain.Status, text string) string {
	if p.Style.Status == nil {
		return text
	}
	return p.Style.Status(s, text)
}

func (p Page) zone() *time.Location {
	if p.Zone == nil {
		return time.UTC
	}
	return p.Zone
}

func width(text string) int { return ansi.StringWidth(text) }

func spaces(n int) string { return strings.Repeat(" ", max(0, n)) }

// left pads plain text on the right to n columns, and right on the left.
func left(text string, n int) string  { return text + spaces(n-width(text)) }
func right(text string, n int) string { return spaces(n-width(text)) + text }

// center pads plain text on both sides to n columns.
func center(text string, n int) string {
	gap := n - width(text)
	return spaces(gap/2) + text + spaces(gap-gap/2)
}

// centered pads text on both sides to n columns and colors only the text,
// so padding never hides inside color codes.
func (p Page) centered(role, text string, n int) string {
	gap := n - width(text)
	return spaces(gap/2) + p.paint(role, text) + spaces(gap-gap/2)
}

// short writes a time in hours and minutes, like 4h12m, leaving seconds
// out so a review stays calm.
func short(ns int64) string {
	minutes := ns / int64(time.Minute)
	if minutes == 0 {
		return "0m"
	}
	return planner.Span(minutes * int64(time.Minute))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func date(text string) time.Time {
	d, _ := time.Parse("2006-01-02", text)
	return d
}

// stat is one labelled number in the grid at the top of a review.
type stat struct{ label, value string }

// grid lays stats out two to a row, in two aligned columns, or one to a
// row when two do not fit.
func (p Page) grid(stats []stat) []string {
	// Only rows with a second stat decide where the second column starts,
	// so one long stat alone on the last row never pushes it out.
	valueWidth := 0
	for i := 0; i+1 < len(stats); i += 2 {
		valueWidth = max(valueWidth, width(stats[i].value))
	}
	cell := func(s stat) string { return p.paint("label", left(s.label, labelWidth)) + s.value }
	var rows []string
	for i := 0; i < len(stats); i += 2 {
		row := margin + cell(stats[i])
		if i+1 < len(stats) {
			next := stats[i+1]
			if labelWidth+valueWidth+4+labelWidth+width(next.value) <= p.room() {
				row += spaces(valueWidth-width(stats[i].value)+4) + cell(next)
			} else {
				rows = append(rows, row)
				row = margin + cell(next)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// barWidth is how many columns a progress bar uses when there is room.
const barWidth = 24

// eighths fill a cell from the left, for the partial end of a bar.
var eighths = []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// bar draws one progress row: a label, a bar of part out of whole, the
// percentage and what the numbers are. A whole of zero draws an empty bar.
func (p Page) bar(label string, part, whole float64, role, detail string) string {
	cells := min(barWidth, max(8, p.room()-labelWidth-9-width(detail)))
	fraction, percent := 0.0, "-"
	if whole > 0 {
		fraction = part / whole
		percent = fmt.Sprintf("%d%%", int(math.Round(fraction*100)))
	}
	filled := min(1, fraction) * float64(cells)
	full := int(filled)
	partial := eighths[int((filled-float64(full))*8)]
	empty := cells - full - width(partial)
	drawn := p.paint(role, strings.Repeat("█", full)+partial) + p.paint("muted", strings.Repeat("░", empty))
	return margin + p.paint("label", left(label, labelWidth)) + drawn + "  " + right(percent, 4) + "   " + p.paint("muted", detail)
}

// progress is the two bars every review has: how much was finished, and how
// much of the planned time was worked.
func (p Page) progress(d planner.DayInsight) []string {
	planned := d.PlannedMinutes * int64(time.Minute)
	return []string{
		p.bar("Completed", float64(d.Finished), float64(d.Total), "good", fmt.Sprintf("%d of %s", d.Finished, plural(d.Total, "item", "items"))),
		p.bar("Worked", float64(d.FocusedNS), float64(planned), "work", fmt.Sprintf("%s of %s planned", short(d.FocusedNS), short(planned))),
	}
}

func interruptions(b domain.Breaks) string {
	if b.Total() == 0 {
		return "none"
	}
	return fmt.Sprintf("%d  (%s, %s)", b.Total(), plural(b.Switches, "switch", "switches"), plural(b.Stops, "stop", "stops"))
}

func average(focused int64, sessions int) string {
	if sessions == 0 {
		return "-"
	}
	return short(focused / int64(sessions))
}

func (p Page) heading(text string) string { return margin + p.paint("heading", text) }

// Day draws one day: its stats, its progress and a timeline of when each
// item ran and waited.
func (p Page) Day(d planner.DayInsight) []string {
	rows := []string{p.heading(date(d.Date).Format("Monday 2 January 2006")), ""}
	if d.Total == 0 {
		return append(rows, margin+p.paint("muted", "Nothing was planned on this day."))
	}
	rows = append(rows, p.grid([]stat{
		{"Focused", short(d.FocusedNS)}, {"Sessions", fmt.Sprint(d.Sessions)},
		{"Paused", short(d.PausedNS)}, {"Average", average(d.FocusedNS, d.Sessions)},
		{"Longest", short(d.LongestNS)}, {"Interrupted", interruptions(d.Breaks)},
	})...)
	rows = append(rows, "")
	rows = append(rows, p.progress(d)...)
	rows = append(rows, "", p.heading("Timeline"))
	return append(rows, p.timeline(d)...)
}

// timeline draws one row per item across the hours the day was worked: a
// full block where it ran and a light one where it waited.
func (p Page) timeline(d planner.DayInsight) []string {
	titleWidth, effortWidth, statusWidth := 0, 0, 0
	var first, last time.Time
	for _, item := range d.Items {
		titleWidth = max(titleWidth, width(item.Title))
		effortWidth = max(effortWidth, width(planner.Effort(item.ItemView)))
		statusWidth = max(statusWidth, width(planner.Label(item.Status)))
		for _, s := range item.Stretches {
			if first.IsZero() || s.From.Before(first) {
				first = s.From
			}
			if s.To.After(last) {
				last = s.To
			}
		}
	}
	titleWidth = min(titleWidth, 24)
	chartWidth := p.room() - titleWidth - 2 - 2 - effortWidth - 2 - statusWidth
	started := !first.IsZero()
	if chartWidth < 12 || !started {
		chartWidth = 0
	}
	var start time.Time
	var span time.Duration
	var rows []string
	slack := 0
	if chartWidth > 0 {
		zone := p.zone()
		f, l := first.In(zone), last.In(zone)
		start = time.Date(f.Year(), f.Month(), f.Day(), f.Hour(), 0, 0, 0, zone)
		end := time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), 0, 0, 0, zone)
		if end.Before(l) {
			end = end.Add(time.Hour)
		}
		if end.Sub(start) < 3*time.Hour {
			end = start.Add(3 * time.Hour)
		}
		span = end.Sub(start)
		// Every hour gets the same number of columns, so the hours on the
		// axis are evenly spaced. What is left over pads the chart's end.
		hours := int(span / time.Hour)
		slack = chartWidth % hours
		chartWidth -= slack
		rows = append(rows, margin+spaces(titleWidth+2)+p.paint("muted", strings.TrimRight(axis(start, span, chartWidth), " ")))
	}
	for _, item := range d.Items {
		title := ansi.Truncate(item.Title, titleWidth, "…")
		row := margin + left(title, titleWidth) + "  "
		if chartWidth > 0 {
			row += p.chart(item.Stretches, start, span, chartWidth) + spaces(slack) + "  "
		}
		row += right(planner.Effort(item.ItemView), effortWidth) + "  " + spaces(statusWidth-width(planner.Label(item.Status))) + p.status(item.Status, planner.Label(item.Status))
		rows = append(rows, row)
	}
	if chartWidth > 0 {
		rows = append(rows, margin+spaces(titleWidth+2)+p.paint("work", "█")+p.paint("muted", " working   ")+p.paint("paused", "░")+p.paint("muted", " paused"))
	} else if !started {
		rows = append(rows, "", margin+p.paint("muted", "Nothing was started on this day."))
	}
	return rows
}

// axis labels the hours of a timeline with two digits, like 09 and 14,
// evenly spaced and as many as fit with room between them. A label that
// would run past the end is left out rather than moved.
func axis(start time.Time, span time.Duration, columns int) string {
	hours := int(span / time.Hour)
	step := 12
	for _, s := range []int{1, 2, 3, 4, 6} {
		if columns*s/hours >= 4 {
			step = s
			break
		}
	}
	line := []rune(spaces(columns))
	free := 0 // the first column a label may use, keeping a gap after the last
	for h := 0; h <= hours; h += step {
		at := columns * h / hours
		if at < free || at+2 > columns {
			continue
		}
		copy(line[at:], []rune(start.Add(time.Duration(h)*time.Hour).Format("15")))
		free = at + 4
	}
	return string(line)
}

// chart draws one item's stretches over columns that each cover an equal
// slice of the span. A slice with any work shows work, unless it waited
// longer than it worked.
func (p Page) chart(stretches []domain.Stretch, start time.Time, span time.Duration, columns int) string {
	kinds := make([]byte, columns)
	for c := range kinds {
		from := start.Add(span * time.Duration(c) / time.Duration(columns))
		to := start.Add(span * time.Duration(c+1) / time.Duration(columns))
		var worked, waited time.Duration
		for _, s := range stretches {
			end, begin := to, from
			if s.To.Before(end) {
				end = s.To
			}
			if s.From.After(begin) {
				begin = s.From
			}
			overlap := end.Sub(begin)
			if overlap <= 0 {
				continue
			}
			if s.Running {
				worked += overlap
			} else {
				waited += overlap
			}
		}
		switch {
		case worked > 0 && worked >= waited:
			kinds[c] = 'w'
		case waited > 0:
			kinds[c] = 'p'
		}
	}
	var b strings.Builder
	for i := 0; i < columns; {
		j := i
		for j < columns && kinds[j] == kinds[i] {
			j++
		}
		switch kinds[i] {
		case 'w':
			b.WriteString(p.paint("work", strings.Repeat("█", j-i)))
		case 'p':
			b.WriteString(p.paint("paused", strings.Repeat("░", j-i)))
		default:
			b.WriteString(spaces(j - i))
		}
		i = j
	}
	return b.String()
}

// Week draws a week from Monday to Sunday: its stats, its progress and a
// bar chart of the hours focused each day.
func (p Page) Week(w planner.PeriodInsight) []string {
	from, to := date(w.From), date(w.To)
	_, number := from.ISOWeek()
	title := fmt.Sprintf("Week %d   %s to %s", number, from.Format("Mon 2 Jan"), to.Format("Mon 2 Jan 2006"))
	total, longestDate, active := w.Sum()
	longest := "-"
	if longestDate != "" {
		longest = short(total.LongestNS) + " on " + date(longestDate).Format("Mon")
	}
	rows := []string{p.heading(title), ""}
	rows = append(rows, p.grid([]stat{
		{"Focused", short(total.FocusedNS)}, {"Active days", fmt.Sprintf("%d of %d", active, len(w.Days))},
		{"Paused", short(total.PausedNS)}, {"Longest", longest},
		{"Daily avg", dailyAverage(total.FocusedNS, active)}, {"Interrupted", interruptions(total.Breaks)},
	})...)
	rows = append(rows, "")
	rows = append(rows, p.progress(total)...)
	rows = append(rows, "", p.heading("Focused hours"), "")
	return append(rows, p.bars(w)...)
}

func dailyAverage(focused int64, active int) string {
	if active == 0 {
		return "-"
	}
	return short(focused / int64(active))
}

// chartHeight is how many rows the week's bar chart is tall.
const chartHeight = 8

// columnWidth is how wide each day of the week's bar chart is: 7 columns
// when there is room, and never fewer than 5.
func (p Page) columnWidth() int { return min(7, max(5, (p.room()-labelWidth)/7)) }

// bars draws the week's focused hours as columns, with an hour scale on
// the left and each day's numbers under its column.
func (p Page) bars(w planner.PeriodInsight) []string {
	columnWidth := p.columnWidth()
	most := 0.0
	for _, d := range w.Days {
		most = max(most, float64(d.FocusedNS)/float64(time.Hour))
	}
	top := max(2, int(math.Ceil(most/2))*2)
	scale := func(h int) string { return fmt.Sprintf("%dh", h) }
	var rows []string
	for r := chartHeight; r >= 1; r-- {
		label, edge := "", " │"
		if r == chartHeight || r == chartHeight/2 {
			label, edge = scale(top*r/chartHeight), " ┤"
		}
		row := margin + p.paint("muted", right(label, labelWidth-2)+edge)
		for _, d := range w.Days {
			level := float64(d.FocusedNS) / float64(time.Hour) / float64(top) * chartHeight
			cell := ""
			switch {
			case level >= float64(r):
				cell = strings.Repeat("█", columnWidth-2)
			case level > float64(r-1):
				cell = strings.Repeat([]string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇"}[int((level-float64(r-1))*8)], columnWidth-2)
			}
			row += " " + p.paint("work", cell) + spaces(columnWidth-1-width(cell))
		}
		rows = append(rows, strings.TrimRight(row, " "))
	}
	rows = append(rows, margin+p.paint("muted", right(scale(0), labelWidth-2)+" └"+strings.Repeat("─", columnWidth*len(w.Days))))
	days, focused, breaks, done := margin+spaces(labelWidth), margin+p.paint("label", left("Focused", labelWidth)), margin+p.paint("label", left("Interrupted", labelWidth)), margin+p.paint("label", left("Completed", labelWidth))
	for _, d := range w.Days {
		role := "muted"
		if d.Date == w.Today {
			role = "today"
		}
		days += p.centered(role, date(d.Date).Format("Mon"), columnWidth)
		if d.Date > w.Today || d.Total == 0 {
			focused += p.centered("muted", "·", columnWidth)
			breaks += p.centered("muted", "·", columnWidth)
			done += p.centered("muted", "·", columnWidth)
			continue
		}
		focused += center(short(d.FocusedNS), columnWidth)
		breaks += center(fmt.Sprint(d.Breaks.Total()), columnWidth)
		done += center(fmt.Sprintf("%d/%d", d.Finished, d.Total), columnWidth)
	}
	trim := func(row string) string { return strings.TrimRight(row, " ") }
	return append(rows, trim(days), "", trim(focused), trim(breaks), trim(done))
}

// Month draws a calendar month: its stats, its progress and a heatmap of
// the hours focused each day, one row per week.
func (p Page) Month(m planner.PeriodInsight) []string {
	total, longestDate, active := m.Sum()
	best := planner.DayInsight{}
	for _, d := range m.Days {
		if d.FocusedNS > best.FocusedNS {
			best = d
		}
	}
	bestDay, longest := "-", "-"
	if best.Active() {
		bestDay = short(best.FocusedNS) + " on " + date(best.Date).Format("Mon 2 Jan")
	}
	if longestDate != "" {
		longest = short(total.LongestNS) + " on " + date(longestDate).Format("Mon 2 Jan")
	}
	rows := []string{p.heading(date(m.From).Format("January 2006")), ""}
	rows = append(rows, p.grid([]stat{
		{"Focused", short(total.FocusedNS)}, {"Active days", fmt.Sprintf("%d of %d", active, len(m.Days))},
		{"Paused", short(total.PausedNS)}, {"Best day", bestDay},
		{"Daily avg", dailyAverage(total.FocusedNS, active)}, {"Longest", longest},
		{"Interrupted", interruptions(total.Breaks)},
	})...)
	rows = append(rows, "")
	rows = append(rows, p.progress(total)...)
	rows = append(rows, "", p.heading("Focus by day"), "")
	return append(rows, p.heatmap(m)...)
}

// cellWidth is how wide each day of the heatmap is: its number, a space,
// two shading blocks and room before the next day, which shrinks to fit.
func (p Page) cellWidth() int { return min(7, max(5, (p.room()-5-6)/7)) }

// shades are the heatmap's levels: none, under 2 hours, 2 to 4, 4 to 6, and
// 6 or more.
var shades = []string{"··", "░░", "▒▒", "▓▓", "██"}

func shade(ns int64) string {
	hours := float64(ns) / float64(time.Hour)
	switch {
	case ns <= 0:
		return shades[0]
	case hours < 2:
		return shades[1]
	case hours < 4:
		return shades[2]
	case hours < 6:
		return shades[3]
	}
	return shades[4]
}

// heatmap draws the month as weeks from Monday to Sunday, with the week
// number on the left and the week's focused hours on the right.
func (p Page) heatmap(m planner.PeriodInsight) []string {
	cellWidth := p.cellWidth()
	names := ""
	for _, name := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		names += left(" "+name, cellWidth)
	}
	rows := []string{margin + spaces(5) + p.paint("muted", strings.TrimRight(names, " "))}
	first := date(m.From)
	offset := (int(first.Weekday()) + 6) % 7
	for start := 0; start < offset+len(m.Days); start += 7 {
		_, number := first.AddDate(0, 0, start-offset).ISOWeek()
		row := margin + p.paint("muted", left(fmt.Sprintf("W%d", number), 5))
		var week int64
		for c := start; c < start+7; c++ {
			i := c - offset
			if i < 0 || i >= len(m.Days) {
				row += spaces(cellWidth)
				continue
			}
			d := m.Days[i]
			number := right(fmt.Sprint(date(d.Date).Day()), 2)
			switch {
			case d.Date > m.Today:
				row += p.paint("muted", number) + spaces(cellWidth-2)
				continue
			case d.Date == m.Today:
				number = p.paint("today", number)
			default:
				number = p.paint("muted", number)
			}
			cell := shade(d.FocusedNS)
			role := "work"
			if cell == shades[0] {
				role = "muted"
			}
			row += number + " " + p.paint(role, cell) + spaces(cellWidth-5)
			week += d.FocusedNS
		}
		total := p.paint("muted", right("-", 6))
		if week > 0 {
			total = right(short(week), 6)
		}
		rows = append(rows, row+total)
	}
	// The legend wraps onto a second row when one is not wide enough.
	rows = append(rows, "")
	line, used := margin+spaces(5), 5
	for i, text := range []string{"none", "under 2h", "2 to 4h", "4 to 6h", "6h or more"} {
		role := "work"
		if i == 0 {
			role = "muted"
		}
		entry := width(shades[i]) + 1 + width(text)
		if used > 5 && used+3+entry > p.room() {
			rows = append(rows, line)
			line, used = margin+spaces(5), 5
		}
		if used > 5 {
			line += "   "
			used += 3
		}
		line += p.paint(role, shades[i]) + p.paint("muted", " "+text)
		used += entry
	}
	return append(rows, line)
}
