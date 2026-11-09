// Package cli connects terminal commands to pure planner rules and SQLite.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

type application struct {
	out io.Writer
	now func() time.Time
}

// Run returns an exit code; only main terminates the process.
func Run(ctx context.Context, args []string, out, stderr io.Writer, version string) int {
	return (application{out: out, now: time.Now}).run(ctx, args, stderr, version)
}

const help = `flowd — a local day planner
Usage: flowd [global options] [command]
       flowd YYYY-MM-DD

Commands:
  list        Show the selected day (default)
  now | next  Show current or next work
  add START END TITLE           Create a block (HH:MM or offset RFC 3339)
  edit [--title T] [--start T] [--end T] BLOCK
  delete BLOCK                  Archive a block; retain its history
  start | pause | resume | finish | skip | reopen BLOCK
  extend BLOCK                  Extend running/paused work by 15 minutes
  events BLOCK                  Read ordered history (full id for archived work)

Starting/resuming/reopening atomically pauses another runner on that day.
Reopen is allowed only on the saved local day. Extension refuses overlap.

BLOCK is a visible row number, full id or unique visible-id prefix.
Rows change after edits/deletion; use full ids for scripts.

Global options (before the command):
  --db PATH             Database path; default: XDG data directory
  --date YYYY-MM-DD     Selected day; default: today
  --timezone ZONE       IANA zone for a new day; saved days retain their zone
  --json                Machine-readable output; errors stay on stderr
  --help, -h            Show help without opening storage
  --version             Show build version

Command flags precede positional arguments. Clock times use the saved day zone.
`

func (app application) run(ctx context.Context, args []string, stderr io.Writer, version string) int {
	opts, err := parseArgs(args)
	if err == nil {
		switch {
		case opts.help:
			_, err = io.WriteString(app.out, help)
		case opts.version:
			if opts.json {
				err = json.NewEncoder(app.out).Encode(map[string]string{"version": version})
			} else {
				_, err = fmt.Fprintln(app.out, "flowd "+version)
			}
		default:
			if opts.zone != "" {
				_, err = loadTimezone(opts.zone)
			}
			if err == nil && opts.path == "" {
				opts.path, err = store.DefaultPath()
			}
			if err == nil {
				opts.path, err = filepath.Abs(opts.path)
			}
			if err == nil {
				err = app.execute(ctx, opts)
			}
		}
	}
	if err == nil {
		return 0
	}
	code, exit, hint := errorDetails(err)
	fmt.Fprintf(stderr, "%s: %s\n", code, clean(err.Error()))
	if opts.path != "" {
		fmt.Fprintf(stderr, "database: %s\n", clean(opts.path))
	}
	if hint != "" {
		fmt.Fprintln(stderr, hint)
	}
	return exit
}

func errorDetails(err error) (string, int, string) {
	switch {
	case errors.Is(err, context.Canceled):
		return "ERR_CANCELLED", 130, ""
	case errors.Is(err, store.ErrCorrupt):
		return "ERR_CORRUPT", 1, "run flowd doctor; restore a known-good backup only with doctor --repair"
	case errors.Is(err, errUsage), errors.Is(err, store.ErrInvalidPath):
		return "ERR_USAGE", 2, "see flowd --help"
	case errors.Is(err, store.ErrLocked):
		return "ERR_LOCKED", 1, "close the other flowd operation and try again"
	case errors.Is(err, store.ErrVersion):
		return "ERR_VERSION", 1, "use a compatible flowd build; do not delete the database"
	case errors.Is(err, store.ErrDiskFull):
		return "ERR_DISK_FULL", 1, "free disk space; earlier committed data is retained"
	case errors.Is(err, store.ErrPermissions), errors.Is(err, os.ErrPermission):
		return "ERR_PERMISSION", 1, "check ownership and database mode 0600"
	case errors.Is(err, domain.ErrOverlap):
		return "ERR_OVERLAP", 1, "choose a free planned interval"
	case errors.Is(err, domain.ErrIllegalTransition):
		return "ERR_ILLEGAL_TRANSITION", 1, ""
	case errors.Is(err, domain.ErrCarryoverRefused):
		return "ERR_CARRYOVER", 1, "only unfinished visible work can be copied"
	case errors.Is(err, domain.ErrInvalidBlockTitle):
		return "ERR_TITLE", 2, "use 1 to 120 characters"
	case errors.Is(err, domain.ErrInvalidStartAndEnd):
		return "ERR_TIME", 2, "end must follow start"
	case errors.Is(err, domain.ErrInvalidDayDate), errors.Is(err, domain.ErrInvalidDayZone):
		return "ERR_USAGE", 2, "check the date and timezone"
	case errors.Is(err, store.ErrBlockNotFound), errors.Is(err, store.ErrDayNotFound), errors.Is(err, os.ErrNotExist):
		return "ERR_NOT_FOUND", 1, "check the selected date, block id and database path"
	case errors.Is(err, store.ErrInvalidEvent), errors.Is(err, store.ErrStaleBlock), errors.Is(err, domain.ErrActiveBlock):
		return "ERR_STATE", 1, "reload the plan and try again"
	default:
		return "ERR_IO", 1, ""
	}
}

func (app application) execute(ctx context.Context, opts options) (err error) {
	database, err := store.Open(ctx, opts.path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, database.Close()) }()
	stamp := app.now().UTC()
	day, zone, err := selectedDay(ctx, database, opts, stamp)
	if err != nil {
		return err
	}
	blocks := make([]domain.Block, 0)
	if day.ID != "" {
		blocks, err = database.BlocksByDay(ctx, day.ID)
		if err != nil {
			return err
		}
	}
	if opts.command == "delete" {
		block, err := resolveBlock(ctx, database, day, blocks, opts.args[0], true)
		if err != nil {
			return err
		}
		if err := database.ArchiveBlock(ctx, block.ID, stamp); err != nil {
			return err
		}
		saved, err := database.BlockByID(ctx, block.ID)
		if err != nil {
			return err
		}
		return printBlock(app.out, opts, "archived", saved, zone)
	}
	if opts.command == "extend" || opts.command == "events" {
		block, err := resolveBlock(ctx, database, day, blocks, opts.args[0], opts.command == "events")
		if err != nil {
			return err
		}
		if opts.command == "events" {
			events, err := database.EventsByBlock(ctx, block.ID)
			if err != nil {
				return err
			}
			if opts.json {
				return json.NewEncoder(app.out).Encode(events)
			}
			for _, event := range events {
				if _, err := fmt.Fprintf(app.out, "%d %s %s -> %s (%s)\n", event.Seq, event.OccurredAt.In(zone).Format(time.RFC3339Nano), event.FromStatus, event.ToStatus, event.Reason); err != nil {
					return err
				}
			}
			return nil
		}
		if err := block.Extend(blocks); err != nil {
			return err
		}
		return app.save(ctx, database, opts, *block, eventFor(*block, block.Status, stamp, domain.ReasonExtend), "extended", zone)
	}
	if opts.command == "edit" {
		return app.edit(ctx, database, opts, day, blocks, zone, stamp)
	}
	if opts.command == "add" {
		return app.add(ctx, database, opts, day, zone, stamp)
	}
	switch opts.command {
	case "start", "pause", "resume", "finish", "skip", "reopen":
		return app.transition(ctx, database, opts, day, blocks, zone, stamp)
	}
	return printView(app.out, opts, makeView(day, blocks, stamp), stamp, zone)
}

// An unsaved empty day is display metadata, not an invented persistent identity.
func selectedDay(ctx context.Context, database *store.Store, opts options, stamp time.Time) (domain.Day, *time.Location, error) {
	name := opts.zone
	if name == "" {
		name = systemTimezone()
	}
	zone, err := loadTimezone(name)
	if err != nil {
		return domain.Day{}, nil, err
	}
	date := opts.date
	if date == "" {
		date = stamp.In(zone).Format("2006-01-02")
	}
	day, err := database.DayByDate(ctx, date)
	if errors.Is(err, store.ErrDayNotFound) {
		return domain.Day{Date: date, Timezone: name}, zone, nil
	}
	if err != nil {
		return domain.Day{}, nil, err
	}
	if opts.zone != "" && opts.zone != day.Timezone {
		return domain.Day{}, nil, usage("day %s already uses %s; omit --timezone to retain it", date, day.Timezone)
	}
	zone, err = time.LoadLocation(day.Timezone)
	if err != nil || day.Timezone == "" || day.Timezone == "Local" {
		return domain.Day{}, nil, fmt.Errorf("%w: invalid saved timezone %q", store.ErrCorrupt, day.Timezone)
	}
	return *day, zone, nil
}
