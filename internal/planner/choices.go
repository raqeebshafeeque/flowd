package planner

import (
	"context"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// Fits tells whether an operation can be offered for an item: a status
// action only when the item's status allows it, and edit and delete for any
// visible item. Past days and clocks are still checked by Do.
func Fits(op Operation, item domain.Item) bool {
	switch op {
	case Edit, Delete:
		return !item.Archived
	case Start, Pause, Resume, Finish, Skip, Unskip, Reopen:
		return item.Can(domain.Action(op))
	}
	return false
}

// Choices lists the visible items of a date, today when date is empty, in
// the day's order, for tab completion. Like Running it reads without the
// lock and never writes, because it runs on every press of Tab.
func (c Client) Choices(ctx context.Context, date string) ([]domain.Item, error) {
	if date == "" {
		date = c.now().In(c.zone()).Format("2006-01-02")
	}
	items, err := store.PeekItems(ctx, c.Path, date)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// Dates lists recently planned dates, newest first, for completing --date.
func (c Client) Dates(ctx context.Context, limit int) ([]string, error) {
	return store.PeekDates(ctx, c.Path, limit)
}
