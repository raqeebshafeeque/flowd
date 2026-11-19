package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/charmbracelet/x/term"
	"github.com/raqeebshafeeque/flowd/internal/cli"
	"github.com/raqeebshafeeque/flowd/internal/planner"
	"github.com/raqeebshafeeque/flowd/internal/store"
	"github.com/raqeebshafeeque/flowd/internal/tui"
)

// version is set when a release is built.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx)
	stop()
	os.Exit(code)
}

// run opens the workspace when flowd is started with no arguments, and the
// command line otherwise.
func run(ctx context.Context) int {
	zone := planner.SystemZone()
	if len(os.Args) > 1 {
		toTerminal := term.IsTerminal(os.Stdout.Fd())
		width := 0
		if toTerminal {
			width, _, _ = term.GetSize(os.Stdout.Fd())
		}
		_, noColor := os.LookupEnv("NO_COLOR")
		env := cli.Env{
			Version:     version,
			Zone:        zone,
			In:          os.Stdin,
			Interactive: term.IsTerminal(os.Stdin.Fd()) && toTerminal,
			Color:       toTerminal && !noColor,
			Width:       width,
		}
		return cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, env)
	}
	path, err := store.DefaultPath()
	if err == nil {
		err = tui.Run(ctx, planner.Client{Path: path, Zone: zone}, os.Stdin, os.Stdout)
	}
	if err != nil {
		return cli.Report(os.Stderr, err)
	}
	return 0
}
