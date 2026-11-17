package tui

import (
	"context"
	"errors"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// ErrNoTerminal means flowd was started without a usable terminal.
var ErrNoTerminal = errors.New("the workspace needs a terminal")

// Run opens the workspace on the given terminal and returns when the person
// quits. The terminal is restored before it returns.
func Run(ctx context.Context, client planner.Client, in, out *os.File) error {
	if !term.IsTerminal(in.Fd()) || !term.IsTerminal(out.Fd()) || os.Getenv("TERM") == "" || os.Getenv("TERM") == "dumb" {
		return planner.Refusal(errors.Join(planner.ErrInput, ErrNoTerminal), "The workspace needs a terminal", "Use flowd list in scripts and pipes")
	}
	m := newModel(ctx, client, client.Zone, time.Now, os.Getenv("NO_COLOR") != "")
	_, err := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx)).Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil
	}
	return err
}
