package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata"

	"github.com/raqeebshafeeque/flowd/internal/cli"
)

// Override with go build -ldflags '-X main.version=...'.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	exit := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, version)
	stop() // os.Exit does not run deferred functions.
	os.Exit(exit)
}
