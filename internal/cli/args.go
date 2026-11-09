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
	command                       string
	args                          []string
	path, date, zone              string
	editTitle, editStart, editEnd string
	json, help, version, repair   bool
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
	case "doctor":
		boolOption(command, "repair", &opts.repair)
	case "list", "review", "watch", "backup", "now", "next", "add", "delete", "start", "pause", "resume", "finish", "skip", "reopen", "extend", "events":
	case "copy":
		stringOption(command, "start", &opts.editStart)
		stringOption(command, "end", &opts.editEnd)
	case "edit":
		stringOption(command, "title", &opts.editTitle)
		stringOption(command, "start", &opts.editStart)
		stringOption(command, "end", &opts.editEnd)
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
	if opts.command == "add" {
		if len(opts.args) < 3 {
			return opts, usage("add needs START END TITLE")
		}
	} else if opts.command != "list" && opts.command != "review" && opts.command != "watch" && opts.command != "doctor" && opts.command != "backup" && opts.command != "now" && opts.command != "next" && opts.command != "edit" {
		if len(opts.args) != 1 {
			return opts, usage("%s needs BLOCK", opts.command)
		}
	} else if opts.command == "edit" {
		if len(opts.args) != 1 || (opts.editTitle == "" && opts.editStart == "" && opts.editEnd == "") {
			return opts, usage("edit needs flags --title/--start/--end followed by BLOCK")
		}
	} else if len(opts.args) != 0 {
		return opts, usage("%s takes no positional arguments", opts.command)
	}
	return opts, nil
}
