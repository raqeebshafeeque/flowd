package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// SettleDue closes every day whose midnight has passed by now: its todo,
// running and paused items become incomplete at exactly midnight. Each day
// closes in its own transaction, once, no matter how many times this runs.
func (s *Store) SettleDue(ctx context.Context, now time.Time) error {
	if !domain.ValidInstant(now) {
		return domain.ErrInvalidEvidence
	}
	// Only days with unfinished items can be due. No timezone is more than
	// a day and a bit ahead of UTC, so later dates cannot have ended yet.
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT d.id FROM items i INDEXED BY unfinished_items JOIN days d ON d.id=i.day_id
 WHERE i.archived=0 AND i.status IN ('todo','in_progress','paused') AND d.date<=?`, now.UTC().Add(48*time.Hour).Format("2006-01-02"))
	if err != nil {
		return classify(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return classify(errors.Join(err, rows.Close()))
		}
		ids = append(ids, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return classify(err)
	}
	for _, id := range ids {
		if err := s.transact(ctx, func(tx *sql.Tx) error { return settleDay(ctx, tx, id, now) }); err != nil {
			return fmt.Errorf("close day %s: %w", id, err)
		}
	}
	return nil
}

func settleDay(ctx context.Context, tx *sql.Tx, id string, now time.Time) error {
	day, err := readDay(ctx, tx, "id", id)
	if err != nil {
		return err
	}
	_, cutoff, err := day.Bounds()
	if err != nil || now.Before(cutoff) {
		return err
	}
	items, err := readItems(ctx, tx, day.ID)
	if err != nil {
		return err
	}
	var changes []Change
	for _, item := range items {
		before := item.Clone()
		changed, err := item.CloseDay(day, now)
		if err != nil {
			return err
		}
		if changed {
			changes = append(changes, Change{item, domain.Record(&before, item, cutoff, now, domain.ReasonDayEnd)})
		}
	}
	return save(ctx, tx, changes)
}
