package cli

import (
	"context"
	"errors"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
	"time"
)

func (app application) copy(ctx context.Context, database *store.Store, opts options, day domain.Day, blocks []domain.Block, zone *time.Location, stamp time.Time) error {
	source, err := resolveBlock(ctx, database, day, blocks, opts.args[0], true)
	if err != nil {
		return err
	}
	if err := source.ValidateCarryOver(); err != nil {
		return err
	}
	date, err := time.Parse("2006-01-02", day.Date)
	if err != nil {
		return err
	}
	targetDate := date.AddDate(0, 0, 1).Format("2006-01-02")
	target, err := database.DayByDate(ctx, targetDate)
	newDay := errors.Is(err, store.ErrDayNotFound)
	if err != nil && !newDay {
		return err
	}
	targetZone := zone
	if newDay {
		target, err = domain.NewDay(targetDate, day.Timezone)
		if err != nil {
			return err
		}
		target.CreatedAt = stamp
	} else {
		loaded, loadedZone, err := selectedDay(ctx, database, options{date: targetDate}, stamp)
		if err != nil {
			return err
		}
		target = &loaded
		targetZone = loadedZone
	}
	start, err := copyClock(source.PlannedStart, zone, targetZone)
	if opts.editStart != "" {
		start, err = parseClock(targetDate, opts.editStart, targetZone, false)
	}
	if err != nil {
		return err
	}
	end, err := copyClock(source.PlannedEnd, zone, targetZone)
	if opts.editEnd != "" {
		end, err = parseClock(targetDate, opts.editEnd, targetZone, true)
	}
	if err != nil {
		return err
	}
	existing := make([]domain.Block, 0)
	if !newDay {
		existing, err = database.BlocksByDay(ctx, target.ID)
		if err != nil {
			return err
		}
	}
	block, err := source.CarryOver(target.ID, start, end, existing)
	if err != nil {
		return err
	}
	block.CreatedAt = stamp
	event := eventFor(*block, block.Status, stamp, domain.ReasonCarryover)
	if newDay {
		err = database.CreateDayWithBlock(ctx, *target, *block, event)
	} else {
		err = database.CreateBlock(ctx, *block, event)
	}
	if err != nil {
		return err
	}
	saved, err := database.BlockByID(ctx, block.ID)
	if err != nil {
		return err
	}
	return printBlock(app.out, opts, "copied", saved, targetZone)
}

// Date arithmetic uses calendar fields, not elapsed 24-hour durations.
func copyClock(value time.Time, source, target *time.Location) (time.Time, error) {
	local := value.In(source)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	shifted, err := parseClock(day.Format("2006-01-02"), local.Format("15:04"), target, false)
	if err != nil {
		return time.Time{}, err
	}
	return shifted.Add(time.Duration(local.Second())*time.Second + time.Duration(local.Nanosecond())), nil
}
