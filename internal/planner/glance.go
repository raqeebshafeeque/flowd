package planner

import (
	"context"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// Running reports the item running today, for a status line. It reads
// without the lock and never writes, so it can be asked every second
// without getting in the way of real commands. ok is false when nothing
// runs. A day whose midnight has passed is not today any more, so its
// running item is not shown, even before the next command closes that day.
func (c Client) Running(ctx context.Context) (item ItemView, ok bool, err error) {
	now := c.now()
	glance, err := store.Peek(ctx, c.Path, now.In(c.zone()).Format("2006-01-02"))
	if err != nil || !glance.Running {
		return ItemView{}, false, err
	}
	if _, end, err := glance.Day.Bounds(); err != nil || !now.Before(end) {
		return ItemView{}, false, err
	}
	metrics, err := domain.Measure(glance.Day, glance.Item, glance.Events, now)
	if err != nil {
		return ItemView{}, false, err
	}
	return ItemView{glance.Item, metrics}, true, nil
}
