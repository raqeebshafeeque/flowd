package cli

import (
	"bufio"
	tea "charm.land/bubbletea/v2"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// licenses holds the license texts of every third-party module built into
// flowd, made by scripts/notices.
//
//go:embed licenses.txt
var licenses string

// Env is what the command line takes from its surroundings.
type Env struct {
	Version string
	Now     func() time.Time // defaults to time.Now
	Zone    *time.Location   // this computer's timezone
	// In is where answers to questions come from, and Interactive says
	// whether a person is at a terminal to answer them.
	In          io.Reader
	Interactive bool
	// Color says whether output may be colored, and Width is the width of
	// the terminal, or zero when output is not a terminal.
	Color bool
	Width int
	// pick runs the item picker. Tests set it to answer without a terminal.
	pick func(tea.Model) (tea.Model, error)
}

// Run executes one command line and returns the exit status. It never exits
// the process itself.
func Run(ctx context.Context, args []string, out, errOut io.Writer, env Env) int {
	if len(args) > 0 && args[0] == "__complete" {
		return complete(ctx, out, args[1:], env)
	}
	o, err := parse(args)
	o.look = look{color: env.Color, width: env.Width}
	if err == nil {
		err = run(ctx, o, out, env)
	}
	if err != nil {
		if o.json || wantsJSON(args) {
			return reportJSON(errOut, err)
		}
		return Report(errOut, err)
	}
	return 0
}

// wantsJSON tells whether --json was asked for, even when the rest of the
// command line could not be read.
func wantsJSON(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--":
			return false
		case "--json", "-json", "--json=true", "-json=true":
			return true
		}
	}
	return false
}

func run(ctx context.Context, o options, out io.Writer, env Env) error {
	switch {
	case o.help:
		return printHelp(out)
	case o.version:
		_, err := fmt.Fprintln(out, "flowd", env.Version)
		return err
	case o.licenses:
		_, err := fmt.Fprint(out, licenses)
		return err
	case o.command == "completion":
		return printCompletion(out, o.args[0])
	case o.command == "skill":
		return installSkill(out, o, env.Version)
	}
	path := o.db
	if path == "" {
		var err error
		if path, err = store.DefaultPath(); err != nil {
			return err
		}
	}
	client := planner.Client{Path: path, Now: env.Now, Zone: env.Zone}
	if o.command == "now" && o.short {
		return statusLine(ctx, out, o, client)
	}
	selection := planner.Selection{Date: o.date, Timezone: o.zone}
	if commands[o.command].positional == "ITEM" && len(o.args) == 0 {
		run := env.pick
		if run == nil {
			run = runPicker(ctx, env, out)
		}
		id, err := choose(ctx, o, client, selection, env, run)
		if err != nil || id == "" {
			return err
		}
		o.args = []string{id}
	}
	switch o.command {
	case "review":
		return review(ctx, out, o, client, env)
	case "list", "now":
		state, err := client.Read(ctx, selection)
		if err != nil {
			return err
		}
		return printDay(out, o, state.View)
	case "add":
		duration, title := o.args[0], joinTitle(o.args[1:])
		r, err := client.Do(ctx, planner.Intent{Operation: planner.Add, Selection: selection, Title: &title, Duration: &duration})
		return finish(out, o, r, err)
	case "events":
		state, item, err := find(ctx, client, selection, o.args[0])
		if err != nil {
			return err
		}
		return printEvents(out, o, state.History[item.ID])
	case "backup":
		return backup(ctx, out, o, path)
	case "doctor":
		return doctor(ctx, out, o, path)
	}
	if o.command == "delete" && !o.yes && (o.json || !env.Interactive) {
		return planner.InputError(nil, "Deleting needs confirmation", "Add --yes to delete without being asked")
	}
	state, item, err := find(ctx, client, selection, o.args[0])
	if err != nil {
		return err
	}
	if o.command == "delete" && !o.yes && !confirm(out, env.In, item.Title) {
		_, err := fmt.Fprintln(out, "Not deleted")
		return err
	}
	intent := planner.Intent{Operation: planner.Operation(o.command), Selection: selection, ItemID: item.ID, Revision: state.Revisions[item.ID]}
	if o.command == "edit" {
		intent.Title, intent.Duration = o.title, o.duration
	}
	r, err := client.Do(ctx, intent)
	return finish(out, o, r, err)
}

// find picks an item by its full ID. Positions in the list are never used,
// because they move when items start, and acting on the wrong item would be
// worse than typing the ID.
func find(ctx context.Context, client planner.Client, selection planner.Selection, id string) (planner.State, planner.ItemView, error) {
	state, err := client.Read(ctx, selection)
	if err != nil {
		return state, planner.ItemView{}, err
	}
	for _, item := range state.View.Items {
		if string(item.ID) == id {
			return state, item, nil
		}
	}
	return state, planner.ItemView{}, planner.Refusal(store.ErrNotFound, "No item "+id+" on "+state.View.Date, "Use the full ID shown by flowd list")
}

// finish prints the result of a change. A confirmed change that could not be
// read back is still reported as saved, so nobody saves it twice.
func finish(out io.Writer, o options, r planner.Result, err error) error {
	if err != nil && r.Outcome != planner.Confirmed {
		if r.Outcome == planner.Unconfirmed {
			return planner.Refusal(errors.Join(planner.ErrStillUnknown, err), "It is unknown whether the change was saved", "Run flowd list to check before trying again")
		}
		return err
	}
	if err != nil {
		return planner.Refusal(err, "The change was saved, but reading it back failed", "Run flowd list to see it")
	}
	return printResult(out, o, r)
}

func backup(ctx context.Context, out io.Writer, o options, path string) error {
	s, err := store.Open(ctx, path, store.WithFullCheck())
	if err != nil {
		return err
	}
	written, err := s.Backup(ctx)
	if err = errors.Join(err, s.Close()); err != nil {
		return err
	}
	if o.json {
		return writeJSON(out, map[string]string{"backup": written})
	}
	_, err = fmt.Fprintln(out, "Backed up to", written)
	return err
}

func doctor(ctx context.Context, out io.Writer, o options, path string) error {
	if o.repair {
		recovery, err := store.Repair(ctx, path)
		if err != nil {
			return err
		}
		if o.json {
			return writeJSON(out, map[string]string{"status": "restored", "recovery": recovery})
		}
		_, err = fmt.Fprintf(out, "Restored the backup. The previous files are in %s\n", recovery)
		return err
	}
	if err := store.Inspect(ctx, path); err != nil {
		return err
	}
	unsafe, err := store.UnsafeTitles(ctx, path)
	if err != nil {
		return err
	}
	if o.json {
		return writeJSON(out, map[string]any{"status": "ok", "unsafe_titles": append([]domain.ID{}, unsafe...)})
	}
	fmt.Fprintln(out, "OK")
	if len(unsafe) > 0 {
		fmt.Fprintf(out, "Warning: %d item titles contain hidden or direction-changing characters.\n", len(unsafe))
		fmt.Fprintln(out, "They are shown with placeholders like <U+202E>. Edit the titles to remove them:")
		for _, id := range unsafe {
			fmt.Fprintln(out, "  "+string(id))
		}
	}
	return nil
}

// joinTitle lets a title be typed without quotes: add 1h Learn Go.
func joinTitle(words []string) string {
	title := words[0]
	for _, w := range words[1:] {
		title += " " + w
	}
	return title
}

// confirm asks whether to delete an item and reads one line. Only "y" or
// "yes" deletes. Anything else, including an empty line or the end of
// input, keeps the item.
func confirm(out io.Writer, in io.Reader, title string) bool {
	fmt.Fprintf(out, "Delete %q? This cannot be undone. [y/N] ", title)
	if in == nil {
		return false
	}
	answer, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}
