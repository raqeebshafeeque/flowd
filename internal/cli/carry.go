package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// carry copies incomplete or skipped items from the last seven days onto
// the day, today unless --date names a later one. The items are named by
// ID, all of them with --all, or picked at a terminal.
func carry(ctx context.Context, out io.Writer, o options, client planner.Client, selection planner.Selection, env Env) error {
	var ids []domain.ID
	for _, arg := range o.args {
		ids = append(ids, domain.ID(arg))
	}
	if len(ids) == 0 {
		left, err := client.Leftovers(ctx, selection)
		if err != nil {
			return err
		}
		if len(left) == 0 {
			if o.json {
				return writeJSON(out, []planner.ItemView{})
			}
			_, err := fmt.Fprintf(out, "Nothing to carry from the last %d days.\n", domain.CarryDays)
			return err
		}
		switch {
		case o.all:
			for _, l := range left {
				ids = append(ids, l.Item.ID)
			}
		case o.json || !env.Interactive:
			return planner.InputError(nil, "Carry needs the IDs of the items, or --all", "Run flowd carry at a terminal to pick them")
		default:
			run := env.pick
			if run == nil {
				run = runPicker(ctx, env, out)
			}
			final, err := run(carryPicker{target: targetDate(o, env), items: left, picked: make([]bool, len(left)), look: o.look})
			if err != nil {
				return err
			}
			if ids = final.(carryPicker).chosenIDs(); len(ids) == 0 {
				return nil
			}
		}
	}
	r, err := client.Do(ctx, planner.Intent{Operation: planner.Carry, Selection: selection, Sources: ids})
	return finish(out, o, r, err)
}

// targetDate is the day items are carried onto: --date, or today.
func targetDate(o options, env Env) string {
	if o.date != "" {
		return o.date
	}
	now := time.Now()
	if env.Now != nil {
		now = env.Now()
	}
	zone := env.Zone
	if zone == nil {
		zone = time.UTC
	}
	return now.In(zone).Format("2006-01-02")
}

// carryPicker lists what can be carried, with a box to tick for each. Up
// and down, or j and k, move, space ticks, a ticks or clears them all,
// Ctrl+S carries what is ticked, like every save, and Esc, q or Ctrl+C
// cancel. It leaves nothing behind when it closes.
type carryPicker struct {
	target  string
	items   []planner.Leftover
	picked  []bool
	cursor  int
	chosen  bool
	closing bool
	look    look
	width   int
}

func (p carryPicker) Init() tea.Cmd { return nil }

func (p carryPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k", "ctrl+p":
			p.cursor = (p.cursor + len(p.items) - 1) % len(p.items)
		case "down", "j", "ctrl+n", "tab":
			p.cursor = (p.cursor + 1) % len(p.items)
		case "space":
			p.picked = toggled(p.picked, p.cursor)
		case "a":
			p.picked = allOrNone(p.picked)
		case "ctrl+s":
			if len(p.chosenTicks()) == 0 {
				return p, nil // nothing ticked, nothing to save
			}
			p.chosen, p.closing = true, true
			return p, tea.Quit
		case "esc", "q", "ctrl+c":
			p.closing = true
			return p, tea.Quit
		}
	}
	return p, nil
}

// toggled ticks or clears one box, without changing the slice it was given.
func toggled(picked []bool, i int) []bool {
	out := append([]bool(nil), picked...)
	out[i] = !out[i]
	return out
}

// allOrNone ticks every box, or clears them all when they are all ticked.
func allOrNone(picked []bool) []bool {
	all := true
	for _, p := range picked {
		all = all && p
	}
	out := make([]bool, len(picked))
	for i := range out {
		out[i] = !all
	}
	return out
}

// chosenIDs is what Ctrl+S carries: the ticked items. A cancelled picker
// carries nothing.
func (p carryPicker) chosenIDs() []domain.ID {
	if !p.chosen {
		return nil
	}
	return p.chosenTicks()
}

func (p carryPicker) chosenTicks() []domain.ID {
	var ids []domain.ID
	for i, on := range p.picked {
		if on {
			ids = append(ids, p.items[i].Item.ID)
		}
	}
	return ids
}

func (p carryPicker) View() tea.View {
	if p.closing {
		return tea.NewView("")
	}
	width := p.width
	if width <= 0 {
		width = 80
	}
	return tea.NewView(strings.Join(leftoverRows(p.items, p.picked, p.cursor, width, p.look, "Carry which items to "+heading(p.target)+"?", "space to pick, a for all, ctrl+s to carry, esc to cancel"), "\n"))
}

// leftoverRows draws a list of leftovers with tick boxes, the day each is
// from, its duration and its status, in aligned columns. The workspace's
// carry panel draws the same rows.
func leftoverRows(items []planner.Leftover, picked []bool, cursor, width int, l look, title, keys string) []string {
	titleWidth, durationWidth, statusWidth := 0, 0, 0
	for _, it := range items {
		duration, _ := domain.FormatDuration(it.Item.DurationMinutes)
		titleWidth = max(titleWidth, ansi.StringWidth(it.Item.Title))
		durationWidth = max(durationWidth, len(duration))
		statusWidth = max(statusWidth, len(planner.Label(it.Item.Status)))
	}
	const dateWidth = len("Wed 18 Nov")
	titleWidth = min(titleWidth, max(12, width-2-4-2-dateWidth-2-durationWidth-2-statusWidth))
	rows := []string{l.paint("\x1b[1m", title) + "  " + l.paint("\x1b[2m", keys)}
	for i, it := range items {
		box := "[ ]"
		if picked[i] {
			box = "[x]"
		}
		name := ansi.Truncate(it.Item.Title, titleWidth, "…")
		name += strings.Repeat(" ", titleWidth-ansi.StringWidth(name))
		duration, _ := domain.FormatDuration(it.Item.DurationMinutes)
		day, _ := time.Parse("2006-01-02", it.Date)
		marker := "  "
		if i == cursor {
			marker = "> "
		}
		left := marker + box + " " + name
		if i == cursor {
			left = l.paint("\x1b[36m", left)
		}
		status := planner.Label(it.Item.Status)
		row := left + "  " + l.paint("\x1b[2m", day.Format("Mon 2 Jan")) + strings.Repeat(" ", dateWidth-len(day.Format("Mon 2 Jan"))) +
			"  " + fmt.Sprintf("%*s", durationWidth, duration) + "  " + strings.Repeat(" ", statusWidth-len(status)) + l.paint(statusColors[it.Item.Status], status)
		rows = append(rows, ansi.Truncate(row, width, ""))
	}
	return rows
}
