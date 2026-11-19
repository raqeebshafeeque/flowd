package cli

import (
	"context"
	"embed"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/planner"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

//go:embed completions/flowd.bash completions/flowd.fish completions/_flowd
var completionScripts embed.FS

// shells maps a shell to the file of its completion script.
var shells = map[string]string{
	"bash": "completions/flowd.bash",
	"fish": "completions/flowd.fish",
	"zsh":  "completions/_flowd",
}

// summaries describe the commands where a shell shows descriptions.
var summaries = map[string]string{
	"list":       "Show the day",
	"add":        "Plan an item",
	"edit":       "Change an item's title or duration",
	"delete":     "Delete an item",
	"start":      "Start an item",
	"pause":      "Pause the running item",
	"resume":     "Continue a paused item",
	"finish":     "Mark an item done for today",
	"skip":       "Drop an item for today",
	"unskip":     "Bring back a skipped item",
	"reopen":     "Keep working on a finished item",
	"now":        "Show the running item",
	"review":     "Show the day's totals and statuses",
	"events":     "Show an item's history",
	"backup":     "Write a backup of the database",
	"doctor":     "Check the database",
	"completion": "Print the tab completion script for a shell",
}

// option is one --name a command accepts, and whether a value follows it.
type option struct {
	name, summary string
	value         bool
}

var globalOptions = []option{
	{"db", "Use another database", true},
	{"date", "Use another day", true},
	{"timezone", "Timezone for a new day", true},
	{"json", "Print JSON", false},
	{"help", "Print the help", false},
}

var commandOptions = map[string][]option{
	"edit":   {{"title", "A new title", true}, {"duration", "A new duration", true}},
	"delete": {{"yes", "Delete without asking", false}},
	"doctor": {{"repair", "Restore the backup", false}},
	"now":    {{"short", "One line for a status bar", false}, {"max", "Longest title for --short", true}},
}

// candidate is one completion: the text to insert and what it means.
type candidate struct{ value, summary string }

// complete answers the hidden __complete command the shell scripts call.
// words are what was typed after flowd, and the last one is the word being
// completed, possibly empty. It prints one candidate per line, the value
// and a tab and its meaning. Like the status line it reads without the lock
// and never writes, and on any problem it simply offers less.
func complete(ctx context.Context, out io.Writer, words []string, env Env) int {
	if len(words) == 0 {
		words = []string{""}
	}
	for _, c := range candidates(ctx, words[:len(words)-1], words[len(words)-1], env) {
		if c.summary == "" {
			fmt.Fprintln(out, c.value)
		} else {
			fmt.Fprintf(out, "%s\t%s\n", c.value, oneLine(c.summary))
		}
	}
	return 0
}

func candidates(ctx context.Context, before []string, current string, env Env) []candidate {
	var command, db, date string
	positional := 0
	for i := 0; i < len(before); i++ {
		word := before[i]
		if name, ok := optionName(word); ok {
			if o, known := findOption(command, name); known && o.value && !strings.Contains(word, "=") {
				if i+1 < len(before) {
					switch name {
					case "db":
						db = before[i+1]
					case "date":
						date = before[i+1]
					}
				}
				i++
			} else if v, has := strings.CutPrefix(word, "--"+name+"="); has {
				switch name {
				case "db":
					db = v
				case "date":
					date = v
				}
			}
			continue
		}
		if command == "" {
			command = word
			if _, err := time.Parse("2006-01-02", word); err == nil {
				command, date = "list", word
			}
			continue
		}
		positional++
	}
	// A value for the option just before the cursor.
	if len(before) > 0 {
		if name, ok := optionName(before[len(before)-1]); ok && !strings.Contains(before[len(before)-1], "=") {
			if o, known := findOption(command, name); known && o.value {
				if name == "date" {
					return matching(dateChoices(ctx, clientFor(db, env), env), current)
				}
				return nil
			}
		}
	}
	if strings.HasPrefix(current, "-") {
		var out []candidate
		for _, o := range append(slices.Clone(globalOptions), commandOptions[command]...) {
			out = append(out, candidate{"--" + o.name, o.summary})
		}
		return matching(out, current)
	}
	if command == "" {
		var out []candidate
		for _, name := range commandNames() {
			out = append(out, candidate{name, summaries[name]})
		}
		return matching(out, current)
	}
	if command == "completion" && positional == 0 {
		return matching([]candidate{{"bash", ""}, {"fish", ""}, {"zsh", ""}}, current)
	}
	if spec, ok := commands[command]; ok && spec.positional == "ITEM" && positional == 0 {
		return matching(itemChoices(ctx, clientFor(db, env), date, command), current)
	}
	return nil
}

func clientFor(db string, env Env) planner.Client {
	if db == "" {
		db, _ = store.DefaultPath()
	}
	return planner.Client{Path: db, Now: env.Now, Zone: env.Zone}
}

// itemChoices are the items a command can act on, with their titles.
func itemChoices(ctx context.Context, client planner.Client, date, command string) []candidate {
	items, err := client.Choices(ctx, date)
	if err != nil {
		return nil
	}
	var out []candidate
	for _, item := range items {
		if command == "events" || planner.Fits(planner.Operation(command), item) {
			out = append(out, candidate{string(item.ID), item.Title})
		}
	}
	return out
}

// dateChoices are today, tomorrow and the days already planned.
func dateChoices(ctx context.Context, client planner.Client, env Env) []candidate {
	now := time.Now()
	if env.Now != nil {
		now = env.Now()
	}
	zone := env.Zone
	if zone == nil {
		zone = time.UTC
	}
	today := now.In(zone)
	out := []candidate{{today.Format("2006-01-02"), "today"}, {today.AddDate(0, 0, 1).Format("2006-01-02"), "tomorrow"}}
	planned, _ := client.Dates(ctx, 30)
	for _, date := range planned {
		if date != out[0].value && date != out[1].value {
			out = append(out, candidate{date, "planned"})
		}
	}
	return out
}

func optionName(word string) (string, bool) {
	name, ok := strings.CutPrefix(word, "--")
	if !ok || name == "" {
		return "", false
	}
	name, _, _ = strings.Cut(name, "=")
	return name, true
}

func findOption(command, name string) (option, bool) {
	for _, o := range append(slices.Clone(globalOptions), commandOptions[command]...) {
		if o.name == name {
			return o, true
		}
	}
	return option{}, false
}

func matching(all []candidate, prefix string) []candidate {
	var out []candidate
	for _, c := range all {
		if strings.HasPrefix(c.value, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// oneLine keeps a title on one line, so it can never break the format the
// shells read.
func oneLine(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, text)
}

// printCompletion prints the completion script for a shell.
func printCompletion(out io.Writer, shell string) error {
	file, ok := shells[shell]
	if !ok {
		return planner.InputError(nil, "There is no completion for "+shell, "Use bash, fish or zsh")
	}
	script, err := completionScripts.ReadFile(file)
	if err != nil {
		return err
	}
	_, err = out.Write(script)
	return err
}
