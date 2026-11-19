package cli

import (
	"context"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// order saves a day's order. Scripts and agents name every item of the day
// once, in the new order. At a terminal, with no IDs, the day can be
// arranged with the keys and saved with Ctrl+S.
func order(ctx context.Context, out io.Writer, o options, client planner.Client, selection planner.Selection, env Env) error {
	var ids []domain.ID
	for _, arg := range o.args {
		ids = append(ids, domain.ID(arg))
	}
	if len(ids) == 0 {
		if o.json || !env.Interactive {
			return planner.InputError(nil, "Order needs the ID of every item, in the new order", "Run flowd order at a terminal to arrange them")
		}
		state, err := client.Read(ctx, selection)
		if err != nil {
			return err
		}
		if len(state.View.Items) < 2 {
			_, err := io.WriteString(out, "There is nothing to put in order.\n")
			return err
		}
		run := env.pick
		if run == nil {
			run = runPicker(ctx, env, out)
		}
		final, err := run(orderPicker{date: state.View.Date, items: state.View.Items, look: o.look})
		if err != nil {
			return err
		}
		p := final.(orderPicker)
		if !p.saved {
			return nil
		}
		for _, item := range p.items {
			ids = append(ids, item.ID)
		}
	}
	r, err := client.Do(ctx, planner.Intent{Operation: planner.Reorder, Selection: selection, Order: ids})
	if err != nil && r.Outcome != planner.Confirmed {
		return finish(out, o, r, err)
	}
	state, err := client.Read(ctx, selection)
	if err != nil {
		return err
	}
	return printDay(out, options{command: "list", json: o.json, look: o.look}, state.View)
}

// orderPicker arranges a day. j and k select an item, J and K move it, and
// Ctrl+S saves. Esc, q or Ctrl+C leave without saving. Nothing is saved
// until Ctrl+S, and it leaves nothing behind when it closes.
type orderPicker struct {
	date    string
	items   []planner.ItemView
	cursor  int
	saved   bool
	closing bool
	look    look
	width   int
}

func (p orderPicker) Init() tea.Cmd { return nil }

func (p orderPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "k", "up":
			p.cursor = max(p.cursor-1, 0)
		case "j", "down":
			p.cursor = min(p.cursor+1, len(p.items)-1)
		case "K", "shift+up":
			p.items, p.cursor = moved(p.items, p.cursor, -1)
		case "J", "shift+down":
			p.items, p.cursor = moved(p.items, p.cursor, 1)
		case "ctrl+s":
			p.saved, p.closing = true, true
			return p, tea.Quit
		case "esc", "q", "ctrl+c":
			p.closing = true
			return p, tea.Quit
		}
	}
	return p, nil
}

// moved swaps the item at i with its neighbour in the direction by, without
// changing the slice it was given, and returns where the item is now.
func moved(items []planner.ItemView, i, by int) ([]planner.ItemView, int) {
	j := i + by
	if j < 0 || j >= len(items) {
		return items, i
	}
	out := append([]planner.ItemView(nil), items...)
	out[i], out[j] = out[j], out[i]
	return out, j
}

func (p orderPicker) View() tea.View {
	if p.closing {
		return tea.NewView("")
	}
	width := p.width
	if width <= 0 {
		width = 80
	}
	effortWidth, statusWidth := 0, 0
	for _, it := range p.items {
		effortWidth = max(effortWidth, ansi.StringWidth(planner.Effort(it)))
		statusWidth = max(statusWidth, len(planner.Label(it.Status)))
	}
	titleWidth := max(12, width-2-2-effortWidth-2-statusWidth-1)
	rows := []string{p.look.paint("\x1b[1m", "Arrange "+heading(p.date)) + "  " + p.look.paint("\x1b[2m", "j/k to select, J/K to move, ctrl+s to save, esc to cancel")}
	for i, it := range p.items {
		name := ansi.Truncate(it.Title, titleWidth, "…")
		name += strings.Repeat(" ", titleWidth-ansi.StringWidth(name))
		left := "  " + name
		if i == p.cursor {
			left = p.look.paint("\x1b[36m", "> "+name)
		}
		effort, status := planner.Effort(it), planner.Label(it.Status)
		row := left + "  " + strings.Repeat(" ", effortWidth-ansi.StringWidth(effort)) + effort + "  " + strings.Repeat(" ", statusWidth-len(status)) + p.look.paint(statusColors[it.Status], status)
		rows = append(rows, ansi.Truncate(row, width, ""))
	}
	return tea.NewView(strings.Join(rows, "\n"))
}
