package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// Env is what the command line takes from its surroundings.
type Env struct {
	Version string
	Now     func() time.Time // defaults to time.Now
	Zone    *time.Location   // this computer's timezone
}

// Run executes one command line and returns the exit status. It never exits
// the process itself.
func Run(ctx context.Context, args []string, out, errOut io.Writer, env Env) int {
	o, err := parse(args)
	if err == nil {
		err = run(ctx, o, out, env)
	}
	if err != nil {
		return Report(errOut, err)
	}
	return 0
}

func run(ctx context.Context, o options, out io.Writer, env Env) error {
	switch {
	case o.help:
		return printHelp(out)
	case o.version:
		_, err := fmt.Fprintln(out, "flowd", env.Version)
		return err
	}
	path := o.db
	if path == "" {
		var err error
		if path, err = store.DefaultPath(); err != nil {
			return err
		}
	}
	client := planner.Client{Path: path, Now: env.Now, Zone: env.Zone}
	selection := planner.Selection{Date: o.date, Timezone: o.zone}
	switch o.command {
	case "list", "review", "now":
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
	state, item, err := find(ctx, client, selection, o.args[0])
	if err != nil {
		return err
	}
	intent := planner.Intent{Operation: planner.Operation(o.command), Selection: selection, ItemID: item.ID, Revision: state.Revisions[item.ID]}
	if o.command == "edit" {
		intent.Title, intent.Duration = o.title, o.duration
	}
	r, err := client.Do(ctx, intent)
	return finish(out, o, r, err)
}

// find picks an item by its row number in the shared order, or by its ID.
func find(ctx context.Context, client planner.Client, selection planner.Selection, ref string) (planner.State, planner.ItemView, error) {
	state, err := client.Read(ctx, selection)
	if err != nil {
		return state, planner.ItemView{}, err
	}
	if n, err := strconv.Atoi(ref); err == nil {
		if n >= 1 && n <= len(state.View.Items) {
			return state, state.View.Items[n-1], nil
		}
	} else {
		for _, item := range state.View.Items {
			if string(item.ID) == ref {
				return state, item, nil
			}
		}
	}
	return state, planner.ItemView{}, planner.Refusal(store.ErrNotFound, "No item "+ref+" on "+state.View.Date, "Use a row number or ID from flowd list")
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
