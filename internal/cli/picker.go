package cli

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/planner"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// done says what an operation did, for "Nothing on Thu 19 Nov 2026 can be
// started".
var done = map[string]string{
	"start": "started", "pause": "paused", "resume": "resumed", "finish": "finished",
	"skip": "skipped", "unskip": "brought back", "reopen": "reopened",
	"edit": "edited", "delete": "deleted", "mark_done": "marked done",
}

// choose asks which item a command is for, when it was given no ID. Only a
// person at a terminal is asked. Scripts, pipes and --json get the same
// refusal as before, so nothing ever waits for an answer that cannot come.
// It returns an empty ID when the person cancels.
func choose(ctx context.Context, o options, client planner.Client, selection planner.Selection, env Env, run func(tea.Model) (tea.Model, error)) (string, error) {
	if o.json || !env.Interactive {
		return "", planner.InputError(nil, capitalize(o.command)+" needs the ID of one item", "Find the ID with flowd list, or leave it out at a terminal to pick one")
	}
	state, err := client.Read(ctx, selection)
	if err != nil {
		return "", err
	}
	var items []planner.ItemView
	for _, item := range state.View.Items {
		if o.command == "events" || planner.Fits(o.op(), item.Item) {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		summary := "Nothing on " + heading(state.View.Date) + " can be " + done[string(o.op())]
		if o.command == "events" {
			summary = "Nothing is planned on " + heading(state.View.Date)
		}
		return "", planner.Refusal(store.ErrNotFound, summary, "See the day with flowd list")
	}
	verb := o.command
	if o.op() == planner.MarkDone {
		verb = "mark done"
	}
	final, err := run(picker{verb: verb, items: items, look: o.look})
	if err != nil {
		return "", err
	}
	p, ok := final.(picker)
	if !ok || !p.chosen {
		return "", nil
	}
	return string(p.items[p.cursor].ID), nil
}

// runPicker shows a picker on the terminal and waits for the answer.
func runPicker(ctx context.Context, env Env, out interface{ Write([]byte) (int, error) }) func(tea.Model) (tea.Model, error) {
	return func(m tea.Model) (tea.Model, error) {
		final, err := tea.NewProgram(m, tea.WithInput(env.In), tea.WithOutput(out), tea.WithContext(ctx)).Run()
		if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
			return m, nil
		}
		return final, err
	}
}

// picker lists the items a command can act on. Up and down, or j and k,
// move, Enter chooses, and Esc, q or Ctrl+C cancel. It draws in place, below
// the prompt, and leaves nothing behind when it closes.
type picker struct {
	verb    string
	items   []planner.ItemView
	cursor  int
	chosen  bool
	closing bool
	look    look
	width   int
}

func (p picker) Init() tea.Cmd { return nil }

func (p picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k", "ctrl+p":
			p.cursor = (p.cursor + len(p.items) - 1) % len(p.items)
		case "down", "j", "ctrl+n", "tab":
			p.cursor = (p.cursor + 1) % len(p.items)
		case "enter":
			p.chosen, p.closing = true, true
			return p, tea.Quit
		case "esc", "q", "ctrl+c":
			p.closing = true
			return p, tea.Quit
		}
	}
	return p, nil
}

func (p picker) View() tea.View {
	if p.closing {
		return tea.NewView("")
	}
	width := p.width
	if width <= 0 {
		width = 80
	}
	titleWidth := 0
	for _, item := range p.items {
		titleWidth = max(titleWidth, ansi.StringWidth(item.Title))
	}
	titleWidth = min(titleWidth, max(12, width-32))
	rows := []string{p.look.paint("\x1b[1m", capitalize(p.verb)+" which item?") + p.look.paint("\x1b[2m", "  ↑↓ to move, enter to choose, esc to cancel")}
	for i, item := range p.items {
		title := ansi.Truncate(item.Title, titleWidth, "…")
		title += strings.Repeat(" ", titleWidth-ansi.StringWidth(title))
		row := "  " + title + "  " + planner.Effort(item) + "  " + p.look.paint(statusColors[item.Status], planner.Label(item.Status))
		if i == p.cursor {
			row = p.look.paint("\x1b[36m", "> "+title) + "  " + planner.Effort(item) + "  " + p.look.paint(statusColors[item.Status], planner.Label(item.Status))
		}
		rows = append(rows, ansi.Truncate(row, width, ""))
	}
	return tea.NewView(strings.Join(rows, "\n"))
}
