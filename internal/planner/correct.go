package planner

import (
	"context"
	"fmt"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// markDone puts right a forgotten finish: an item left incomplete on one of
// the last seven days becomes done. Only its status
// changes. Its work stays what was tracked, nothing is invented, and the
// correction is recorded with when it was made.
func (s *session) markDone(ctx context.Context, intent Intent) (Result, error) {
	today := s.now.In(s.zone).Format("2006-01-02")
	if s.day.Date >= today {
		return Result{}, Refusal(domain.ErrIllegalTransition, "Only items on past days are marked done later", "Finish it the usual way with f or flowd finish")
	}
	d, _ := time.Parse("2006-01-02", today)
	if earliest := d.AddDate(0, 0, -domain.RecentDays).Format("2006-01-02"); s.day.Date < earliest {
		return Result{}, Refusal(ErrPastDay, fmt.Sprintf("Only the last %d days can be put right", domain.RecentDays), "Older days stay as they were")
	}
	item, err := s.current(ctx, intent.ItemID, intent.Revision)
	if err != nil {
		return Result{}, err
	}
	before := item.Clone()
	if err := item.CorrectToDone(s.now); err != nil {
		return Result{}, err
	}
	event := domain.Record(&before, item, s.now, s.now, domain.ReasonUser)
	return s.persist(ctx, item, nil, func() error { return s.db.Save(ctx, store.Change{Item: item, Event: event}) })
}
