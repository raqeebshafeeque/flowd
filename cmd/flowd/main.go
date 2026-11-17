package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/raqeebshafeeque/flowd/internal/cli"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// version is set when a release is built.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, cli.Env{Version: version, Zone: planner.SystemZone()})
	stop()
	os.Exit(code)
}
