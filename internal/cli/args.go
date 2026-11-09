package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"
)

var errUsage = errors.New("invalid command")

type options struct {
	command             string
	args                []string
	path, date, zone    string
	json, help, version bool
}

func usage(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUsage, fmt.Sprintf(format, args...))
}

func flags(name string) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return set
}

func stringOption(set *flag.FlagSet, name string, target *string) {
	seen := false
	set.Func(name, name, func(value string) error {
		if seen {
			return fmt.Errorf("--%s was supplied twice", name)
		}
		seen = true
		if value == "" {
			return fmt.Errorf("--%s cannot be empty", name)
		}
		*target = value
		return nil
	})
}

func boolOption(set *flag.FlagSet, name string, target *bool) {
	seen := false
	set.BoolFunc(name, name, func(value string) error {
		if seen {
			return fmt.Errorf("--%s was supplied twice", name)
		}
		seen = true
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		*target = parsed
		return nil
	})
}

func parseArgs(args []string) (options, error) {
	var opts options
	global := flags("flowd")
	stringOption(global, "db", &opts.path)
	stringOption(global, "date", &opts.date)
	stringOption(global, "timezone", &opts.zone)
	boolOption(global, "json", &opts.json)
	boolOption(global, "help", &opts.help)
	boolOption(global, "h", &opts.help)
	boolOption(global, "version", &opts.version)
	if err := global.Parse(args); err != nil {
		return opts, usage("%v", err)
	}
	if opts.help || opts.version {
		return opts, nil
	}
	positional := global.Args()
	opts.command = "list"
	if len(positional) > 0 {
		opts.command = positional[0]
		opts.args = positional[1:]
		if _, err := time.Parse("2006-01-02", opts.command); err == nil {
			if opts.date != "" {
				return opts, usage("choose a date only once")
			}
			opts.date, opts.command = opts.command, "list"
		}
	}
	if opts.date != "" {
		if _, err := time.Parse("2006-01-02", opts.date); err != nil {
			return opts, usage("invalid date %q; use YYYY-MM-DD", opts.date)
		}
	}
	command := flags(opts.command)
	boolOption(command, "help", &opts.help)
	boolOption(command, "h", &opts.help)
	switch opts.command {
	case "list", "now", "next":
	default:
		return opts, usage("unknown command %q", opts.command)
	}
	if err := command.Parse(opts.args); err != nil {
		return opts, usage("%v", err)
	}
	if opts.help {
		return opts, nil
	}
	opts.args = command.Args()
	if len(opts.args) != 0 {
		return opts, usage("%s takes no positional arguments", opts.command)
	}
	return opts, nil
}
