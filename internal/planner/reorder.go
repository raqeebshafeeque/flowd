package planner

import (
	"context"
	"slices"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// reorder saves a new order for the day on screen. The order has to name
// every visible item exactly once. If the day changed since it was read, by
// an item added or deleted, nothing is saved. The order is how the list is
// laid out, not part of any item's history, so it records no events.
func (s *session) reorder(ctx context.Context, intent Intent) (Result, error) {
	if err := s.requirePlanning(); err != nil {
		return Result{}, err
	}
	result := Result{Outcome: Refused, At: s.now, Day: s.day, Order: slices.Clone(intent.Order)}
	if result.Order == nil {
		result.Order = []domain.ID{}
	}
	if s.day.ID == "" {
		if len(intent.Order) > 0 {
			return result, Refusal(store.ErrOrderChanged, "The day changed while it was being reordered", "Reorder it again")
		}
		result.Outcome = Confirmed
		return result, nil
	}
	if err := s.db.SetOrder(ctx, s.day.ID, intent.Order); err != nil {
		if !refusal(err) {
			result.Outcome = Unconfirmed
		}
		return result, err
	}
	result.Outcome = Confirmed
	return result, nil
}

// reconcileOrder settles a reorder whose save might not have landed: it did
// if the day is now in the order that was sent.
func (s *session) reconcileOrder(ctx context.Context, pending Result) (Result, error) {
	items, err := s.db.Items(ctx, s.day.ID)
	if err != nil {
		return pending, err
	}
	now := make([]domain.ID, len(items))
	for i, item := range items {
		now[i] = item.ID
	}
	pending.Outcome = Refused
	if slices.Equal(now, pending.Order) {
		pending.Outcome = Confirmed
	}
	return pending, nil
}
