package cli

import (
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// options is a parsed command line.
type options struct {
	command  string
	args     []string
	db       string
	date     string
	zone     string
	json     bool
	help     bool
	version  bool
	licenses bool
	repair   bool
	yes      bool
	title    *string
	duration *string
}

// command describes how a command takes its arguments.
type command struct {
	positional string // "", "ITEM" or "DURATION TITLE"
	flags      func(*flag.FlagSet, *options)
}

var commands = map[string]command{
	"list":   {},
	"review": {},
	"now":    {},
	"backup": {},
	"doctor": {flags: func(f *flag.FlagSet, o *options) { f.BoolVar(&o.repair, "repair", false, "") }},
	"add":    {positional: "DURATION TITLE"},
	"edit": {positional: "ITEM", flags: func(f *flag.FlagSet, o *options) {
		f.Func("title", "", func(v string) error { o.title = &v; return nil })
		f.Func("duration", "", func(v string) error { o.duration = &v; return nil })
	}},
	"delete": {positional: "ITEM", flags: func(f *flag.FlagSet, o *options) { f.BoolVar(&o.yes, "yes", false, "") }},
	"start":  {positional: "ITEM"},
	"pause":  {positional: "ITEM"},
	"resume": {positional: "ITEM"},
	"finish": {positional: "ITEM"},
	"skip":   {positional: "ITEM"},
	"unskip": {positional: "ITEM"},
	"reopen": {positional: "ITEM"},
	"events": {positional: "ITEM"},
}

// commandNames lists every command, sorted.
func commandNames() []string {
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func newFlags(name string) *flag.FlagSet {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	return f
}

func usage(summary string) error {
	return planner.InputError(nil, summary, "See flowd --help for every command and option")
}

// globals adds the options every command accepts. They can come before the
// command, after it or after its arguments, but each value can be given
// only once, so a later one never quietly wins over an earlier one.
func globals(f *flag.FlagSet, o *options, seen map[string]int) {
	value := func(name string, target *string) {
		f.Func(name, "", func(v string) error {
			seen[name]++
			*target = v
			return nil
		})
	}
	value("db", &o.db)
	value("date", &o.date)
	value("timezone", &o.zone)
	f.BoolVar(&o.json, "json", o.json, "")
	f.BoolVar(&o.help, "help", o.help, "")
	f.BoolVar(&o.help, "h", o.help, "")
	f.BoolVar(&o.version, "version", o.version, "")
	f.BoolVar(&o.licenses, "licenses", o.licenses, "")
}

// parseAll reads options wherever they appear among the arguments and
// returns the rest in order. Everything after -- is an argument.
func parseAll(f *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for {
		if err := f.Parse(args); err != nil {
			return nil, err
		}
		left := f.Args()
		if used := len(args) - len(left); used > 0 && args[used-1] == "--" {
			return append(rest, left...), nil
		}
		if len(left) == 0 {
			return rest, nil
		}
		rest, args = append(rest, left[0]), left[1:]
	}
}

// parse reads the command line. Options can appear anywhere. The first
// argument that is not an option is the command, and a bare date such as
// 2026-11-17 means list for that date.
func parse(args []string) (options, error) {
	var o options
	seen := map[string]int{}
	global := newFlags("flowd")
	globals(global, &o, seen)
	if err := global.Parse(args); err != nil {
		return o, optionError(err)
	}
	rest := global.Args()
	if used := len(args) - len(rest); used > 0 && args[used-1] == "--" {
		return o, usage("Put the command before --")
	}
	o.command = "list"
	if len(rest) > 0 {
		o.command, rest = rest[0], rest[1:]
	}
	if _, err := time.Parse("2006-01-02", o.command); err == nil {
		seen["date"]++
		o.date, o.command = o.command, "list"
	}
	spec, ok := commands[o.command]
	if !ok && !(o.help || o.version || o.licenses) {
		return o, usage("There is no command called " + o.command)
	}
	local := newFlags(o.command)
	globals(local, &o, seen)
	if spec.flags != nil {
		spec.flags(local, &o)
	}
	var err error
	if o.args, err = parseAll(local, rest); err != nil {
		return o, optionError(err)
	}
	for _, name := range []string{"db", "date", "timezone"} {
		if seen[name] > 1 {
			return o, usage("Give the " + name + " only once")
		}
	}
	if o.help || o.version || o.licenses {
		return o, nil
	}
	if o.date != "" {
		if d, err := time.Parse("2006-01-02", o.date); err != nil || d.Format("2006-01-02") != o.date {
			return o, planner.InputError(nil, "There is no date "+o.date, "Use a date like 2026-11-17")
		}
	}
	switch spec.positional {
	case "":
		if len(o.args) != 0 {
			return o, usage(capitalize(o.command) + " takes no arguments")
		}
	case "ITEM":
		if len(o.args) != 1 {
			return o, planner.InputError(nil, capitalize(o.command)+" needs the ID of one item", "Find the ID with flowd list")
		}
	case "DURATION TITLE":
		if len(o.args) < 2 {
			return o, planner.InputError(nil, "Add needs a duration and a title", `For example: flowd add 1h30m "Study"`)
		}
	}
	if o.command == "edit" && o.title == nil && o.duration == nil {
		return o, planner.InputError(nil, "Edit needs a new title, a new duration or both", "Use --title, --duration or both")
	}
	return o, nil
}

// optionError rewrites the flag package's errors in plain words, naming
// the option the way it is written on the command line.
func optionError(err error) error {
	text := err.Error()
	for _, known := range []struct{ prefix, words string }{
		{"flag provided but not defined: -", "There is no option --"},
		{"flag needs an argument: -", "This option needs a value: --"},
	} {
		if name, ok := strings.CutPrefix(text, known.prefix); ok {
			return usage(known.words + strings.TrimPrefix(name, "-"))
		}
	}
	return usage(capitalize(text))
}

func capitalize(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

const help = `flowd plans your day as a list of efforts and records the work you do.

Usage:
  flowd [command] [options]
  flowd <YYYY-MM-DD> [options]    Show that date

Commands:
  list                         Show the day (the default)
  add <DURATION> <TITLE>       Plan an item, for example: add 1h30m "Study"
  edit [--title <TITLE>] [--duration <DURATION>] <ID>
  delete [--yes] <ID>          Delete an item, after asking. --yes skips the question
  start <ID>                   Start an item, pausing whatever was running
  pause <ID>                   Pause the running item
  resume <ID>                  Continue a paused item
  finish <ID>                  Mark an item done for today
  skip <ID>                    Drop an item for today
  unskip <ID>                  Bring back a skipped item
  reopen <ID>                  Keep working on a finished item
  now                          Show the running item
  review                       Show the day's totals, progress and statuses
  events <ID>                  Show an item's recorded history
  backup                       Write a backup next to the database
  doctor [--repair]            Check the database, or restore the backup

<ID> is the full ID of an item, as shown by list, like it_01JD8K6Z2KAS98N1W1N1PMM62G.
<DURATION> is whole minutes or hours: 30, 30m, 2h or 1h30m.
Options can go before or after the command. Put -- before a title that starts with a dash.

Options for every command:
  --db <PATH>                  Use this database instead of the default one
  --date <YYYY-MM-DD>          Use this date instead of today
  --timezone <ZONE>            Timezone for a new day, like Asia/Kolkata
  --json                       Print JSON for scripts and agents
  --version                    Print the version
  --licenses                   Print the licenses of the code flowd is built with
  --help                       Print this help
`

func printHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, help)
	return err
}
