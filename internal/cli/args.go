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

func usage(summary string) error { return planner.InputError(nil, summary, "See flowd --help") }

// parse reads global options, then the command and its own options, then
// its positional arguments. A bare date such as 2026-11-17 means list.
func parse(args []string) (options, error) {
	var o options
	global := newFlags("flowd")
	global.StringVar(&o.db, "db", "", "")
	global.StringVar(&o.date, "date", "", "")
	global.StringVar(&o.zone, "timezone", "", "")
	global.BoolVar(&o.json, "json", false, "")
	global.BoolVar(&o.help, "help", false, "")
	global.BoolVar(&o.help, "h", false, "")
	global.BoolVar(&o.version, "version", false, "")
	global.BoolVar(&o.licenses, "licenses", false, "")
	if err := global.Parse(args); err != nil {
		return o, usage(sentence(err))
	}
	if o.help || o.version || o.licenses {
		return o, nil
	}
	rest := global.Args()
	o.command = "list"
	if len(rest) > 0 {
		o.command, rest = rest[0], rest[1:]
	}
	if _, err := time.Parse("2006-01-02", o.command); err == nil {
		if o.date != "" {
			return o, usage("Give the date only once")
		}
		o.date, o.command = o.command, "list"
	}
	if o.date != "" {
		if d, err := time.Parse("2006-01-02", o.date); err != nil || d.Format("2006-01-02") != o.date {
			return o, planner.InputError(nil, "Invalid date "+o.date, "Use a date like 2026-11-17")
		}
	}
	spec, ok := commands[o.command]
	if !ok {
		return o, usage("Unknown command " + o.command)
	}
	local := newFlags(o.command)
	local.BoolVar(&o.help, "help", false, "")
	local.BoolVar(&o.help, "h", false, "")
	if spec.flags != nil {
		spec.flags(local, &o)
	}
	if err := local.Parse(rest); err != nil {
		return o, usage(sentence(err))
	}
	if o.help {
		return o, nil
	}
	o.args = local.Args()
	switch spec.positional {
	case "":
		if len(o.args) != 0 {
			return o, usage(o.command + " takes no arguments")
		}
	case "ITEM":
		if len(o.args) != 1 {
			return o, usage(o.command + " needs the ID of one item")
		}
	case "DURATION TITLE":
		if len(o.args) < 2 {
			return o, usage("add needs a <DURATION> and a <TITLE>")
		}
	}
	if o.command == "edit" && o.title == nil && o.duration == nil {
		return o, usage("edit needs --title, --duration or both")
	}
	return o, nil
}

// sentence turns a flag package error into a capitalised sentence.
func sentence(err error) string {
	text := err.Error()
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

const help = `flowd plans your day as a list of efforts and records the work you do.

Usage:
  flowd [options] [command]
  flowd [options] <YYYY-MM-DD>    Show that date

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

Options:
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
