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
	json     bool
	help     bool
	version  bool
	licenses bool
	repair   bool
	short    bool
	agents   map[string]bool // for skill: which agents to install it for
	all      bool            // for carry: every leftover
	// operation overrides what the command does, when it means something
	// else in context, like finish on a past day.
	operation planner.Operation
	yes       bool
	noBackup  bool
	title     *string
	duration  *string
	look      look // set from Env, not from the command line
}

// command describes how a command takes its arguments.
type command struct {
	positional string // "", "ITEM" or "DURATION TITLE"
	flags      func(*flag.FlagSet, *options)
}

var commands = map[string]command{
	"list":   {},
	"review": {},
	"now":    {flags: func(f *flag.FlagSet, o *options) { f.BoolVar(&o.short, "short", false, "") }},
	"backup": {},
	"reset": {flags: func(f *flag.FlagSet, o *options) {
		f.BoolVar(&o.yes, "yes", false, "")
		f.BoolVar(&o.noBackup, "no-backup", false, "")
	}},
	"doctor": {flags: func(f *flag.FlagSet, o *options) { f.BoolVar(&o.repair, "repair", false, "") }},
	"add":    {positional: "DURATION TITLE"},
	"edit": {positional: "ITEM", flags: func(f *flag.FlagSet, o *options) {
		f.Func("title", "", func(v string) error { o.title = &v; return nil })
		f.Func("duration", "", func(v string) error { o.duration = &v; return nil })
	}},
	"delete":     {positional: "ITEM", flags: func(f *flag.FlagSet, o *options) { f.BoolVar(&o.yes, "yes", false, "") }},
	"start":      {positional: "ITEM"},
	"pause":      {positional: "ITEM"},
	"finish":     {positional: "ITEM"},
	"completion": {positional: "SHELL"},
	"order":      {positional: "IDS"},
	"carry": {positional: "IDS", flags: func(f *flag.FlagSet, o *options) {
		f.BoolVar(&o.all, "all", false, "")
	}},
	"skill": {flags: func(f *flag.FlagSet, o *options) {
		o.agents = map[string]bool{}
		for _, a := range agents {
			f.BoolFunc(a.flag, "", func(string) error { o.agents[a.flag] = true; return nil })
		}
	}},
}

// op is the planner operation the command line asks for.
func (o options) op() planner.Operation {
	if o.operation != "" {
		return o.operation
	}
	return planner.Operation(o.command)
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

// parse reads the command line. Options can appear anywhere, and the first
// argument that is not an option is the command.
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
	for _, name := range []string{"db", "date"} {
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
		// No ID at all is allowed here. At a terminal it opens the picker,
		// and anywhere else run refuses it.
		if len(o.args) > 1 {
			return o, planner.InputError(nil, capitalize(o.command)+" takes the ID of one item", "Find the ID with flowd list")
		}
	case "IDS":
		if o.all && len(o.args) > 0 {
			return o, planner.InputError(nil, "Carry either the items named or --all, not both", "Use flowd carry --all, or name the IDs")
		}
	case "SHELL":
		if len(o.args) != 1 {
			return o, planner.InputError(nil, "Completion needs a shell", "Use flowd completion bash, fish or zsh")
		}
	case "DURATION TITLE":
		if len(o.args) < 2 {
			return o, planner.InputError(nil, "Add needs a duration and a title", `For example: flowd add 1h30m "Study"`)
		}
	}
	if o.command == "skill" && len(o.agents) == 0 {
		return o, planner.InputError(nil, "Choose which agents get the skill", "Use --claude, --codex or both")
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

Plan the day:
  list                         Show the day (the default)
  add <DURATION> <TITLE>       Plan an item, for example: add 1h30m "Study"
  edit [--title <TITLE>] [--duration <DURATION>] <ID>
  delete [--yes] <ID>          Delete an item, after asking. --yes skips the question
  order [<ID>...]              Put the day in this order, or arrange it at a terminal
  carry [<ID>...] [--all]      Copy incomplete items from the last 7 days onto the day

Track the work:
  start <ID>                   Start, resume or reopen an item, pausing whatever was running
  pause <ID>                   Pause the running item
  finish <ID>                  Mark an item done. On a past day, put a forgotten finish right
  now [--short]                Show the running item. --short prints one line for a status bar
  review                       Show the day's totals, progress and statuses

Set up and look after:
  completion <SHELL>           Print tab completion for bash, fish or zsh
  skill [--claude] [--codex]   Install the Agent Skill for Claude Code, Codex or both
  backup                       Write a backup next to the database
  doctor [--repair]            Check the database, or restore the backup
  reset [--yes] [--no-backup]  Start over with an empty planner. Asks whether to keep the old files

<ID> is the full ID of an item, as shown by list, like it_01JD8K6Z2KAS98N1W1N1PMM62G. Leave it
out at a terminal to pick the item with the arrow keys, or press Tab with completion set up.
<DURATION> is whole minutes or hours up to a day: 30, 30m, 2h or 1h30m.
Options can go before or after the command. Put -- before a title that starts with a dash.

Options for every command:
  --date <YYYY-MM-DD>          Use this date instead of today
  --db <PATH>                  Use this database instead of the default one
  --json                       Print JSON for scripts and agents
  --version                    Print the version
  --licenses                   Print the licenses of the code flowd is built with
  --help                       Print this help
`

func printHelp(out io.Writer) error {
	_, err := fmt.Fprint(out, help)
	return err
}
