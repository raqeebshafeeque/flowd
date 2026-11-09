package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
	"github.com/raqeebshafeeque/flowd/internal/utils"
)

func titleText(text string) (string, error) {
	text = strings.TrimSpace(text)
	for _, character := range text {
		if unicode.IsControl(character) {
			return "", usage("titles must be single-line text without control characters")
		}
	}
	return text, nil
}

func eventFor(block domain.Block, from domain.Status, stamp time.Time, reason domain.EventReason) domain.BlockEvent {
	return domain.BlockEvent{BlockID: block.ID, FromStatus: from, ToStatus: block.Status, OccurredAt: stamp, Reason: reason}
}

func (app application) add(ctx context.Context, database *store.Store, opts options, day domain.Day, zone *time.Location, stamp time.Time) error {
	title, err := titleText(strings.Join(opts.args[2:], " "))
	if err != nil {
		return err
	}
	start, err := parseClock(day.Date, opts.args[0], zone, false)
	if err != nil {
		return err
	}
	end, err := parseClock(day.Date, opts.args[1], zone, true)
	if err != nil {
		return err
	}
	newDay := day.ID == ""
	if newDay {
		created, err := domain.NewDay(day.Date, day.Timezone)
		if err != nil {
			return err
		}
		day = *created
		day.CreatedAt = stamp
	}
	block, err := domain.NewBlock(domain.NewBlockInput{Title: title, DayID: day.ID, PlannedStartAt: start, PlannedEndAt: end})
	if err != nil {
		return err
	}
	block.CreatedAt = stamp
	event := eventFor(*block, block.Status, stamp, domain.ReasonUser)
	if newDay {
		err = database.CreateDayWithBlock(ctx, day, *block, event)
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
	return printBlock(app.out, opts, "added", saved, zone)
}

func (app application) edit(ctx context.Context, database *store.Store, opts options, day domain.Day, blocks []domain.Block, zone *time.Location, stamp time.Time) error {
	block, err := resolveBlock(ctx, database, day, blocks, opts.args[0], false)
	if err != nil {
		return err
	}
	if opts.editTitle != "" {
		block.Title, err = titleText(opts.editTitle)
		if err != nil {
			return err
		}
	}
	if opts.editStart != "" {
		block.PlannedStart, err = parseClock(day.Date, opts.editStart, zone, false)
		if err != nil {
			return err
		}
	}
	if opts.editEnd != "" {
		block.PlannedEnd, err = parseClock(day.Date, opts.editEnd, zone, true)
		if err != nil {
			return err
		}
	}
	input := domain.NewBlockInput{Title: block.Title, PlannedStartAt: block.PlannedStart, PlannedEndAt: block.PlannedEnd}
	if err := input.Validate(); err != nil {
		return err
	}
	return app.save(ctx, database, opts, *block, eventFor(*block, block.Status, stamp, domain.ReasonUser), "edited", zone)
}

func (app application) save(ctx context.Context, database *store.Store, opts options, block domain.Block, event domain.BlockEvent, action string, zone *time.Location) error {
	if err := database.SaveMoves(ctx, []store.BlockChange{{Block: block, Event: event}}); err != nil {
		return err
	}
	saved, err := database.BlockByID(ctx, block.ID)
	if err != nil {
		return err
	}
	return printBlock(app.out, opts, action, saved, zone)
}

func printBlock(out io.Writer, opts options, action string, block *domain.Block, zone *time.Location) error {
	if opts.json {
		return json.NewEncoder(out).Encode(block)
	}
	_, err := fmt.Fprintf(out, "%s: %s\n", action, describe(block, zone))
	return err
}

func resolveBlock(ctx context.Context, database *store.Store, day domain.Day, blocks []domain.Block, selector string, archived bool) (*domain.Block, error) {
	if strings.TrimSpace(selector) == "" {
		return nil, usage("block number or id cannot be empty")
	}
	if number, err := strconv.Atoi(selector); err == nil {
		if number > 0 && number <= len(blocks) {
			block := blocks[number-1]
			return &block, nil
		}
		return nil, fmt.Errorf("%w: no row %s on %s", store.ErrBlockNotFound, selector, day.Date)
	}
	block, err := database.BlockByID(ctx, utils.ID(selector))
	if err == nil {
		if block.DayID != day.ID {
			return nil, fmt.Errorf("%w: id %q is not on %s", store.ErrBlockNotFound, selector, day.Date)
		}
		if block.Archived && !archived {
			return nil, fmt.Errorf("%w: block is archived", domain.ErrIllegalTransition)
		}
		return block, nil
	}
	if !errors.Is(err, store.ErrBlockNotFound) {
		return nil, err
	}
	var match *domain.Block
	for _, candidate := range blocks {
		if strings.HasPrefix(string(candidate.ID), selector) {
			if match != nil {
				return nil, usage("id prefix %q is ambiguous; use a longer id", selector)
			}
			copy := candidate
			match = &copy
		}
	}
	if match == nil {
		return nil, fmt.Errorf("%w: %q on %s", store.ErrBlockNotFound, selector, day.Date)
	}
	return match, nil
}
