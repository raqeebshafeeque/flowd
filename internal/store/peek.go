package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// Glance is what a peek found: the day of a date and the item running on
// it, with its history. Running is false when nothing runs, and Day is
// empty when the date was never planned.
type Glance struct {
	Day     domain.Day
	Item    domain.Item
	Events  []domain.Event
	Running bool
}

// Peek reads the running item of a date for a status line, which may ask
// every second or two. Unlike Open it takes no lock, so it never makes a
// real command wait or fail. It never writes, never creates a file or a
// folder and never closes a day. In WAL mode a reader sees the last saved
// state as a whole, even while another flowd saves, so no lock is needed
// to read safely. Only the schema is checked. Looking for damage is left to
// the commands that open the file properly.
func Peek(ctx context.Context, path, date string) (_ Glance, err error) {
	var glance Glance
	err = peek(ctx, path, func(tx *sql.Tx) error {
		day, err := readDay(ctx, tx, "date", date)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		glance.Day = day
		item, err := scanItem(tx.QueryRowContext(ctx, "SELECT "+itemColumns+" FROM items WHERE day_id=? AND archived=0 AND status=?", day.ID, domain.InProgress))
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		events, err := readEvents(ctx, tx, "item_id=?", item.ID)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return fmt.Errorf("%w: item %s has no history", ErrCorrupt, item.ID)
		}
		glance.Item, glance.Events, glance.Running = item, events, true
		return nil
	})
	if err != nil {
		return Glance{}, err
	}
	return glance, nil
}

// PeekItems reads the visible items of a date the same way as Peek, with no
// lock and no writes, for tab completion. A date never planned has none.
func PeekItems(ctx context.Context, path, date string) (items []domain.Item, err error) {
	err = peek(ctx, path, func(tx *sql.Tx) error {
		day, err := readDay(ctx, tx, "date", date)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		items, err = readItems(ctx, tx, day.ID)
		return err
	})
	return items, err
}

// PeekDates reads the most recent planned dates, newest first, the same way
// as Peek, for completing --date.
func PeekDates(ctx context.Context, path string, limit int) (dates []string, err error) {
	err = peek(ctx, path, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT date FROM days ORDER BY date DESC LIMIT ?", limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var date string
			if err := rows.Scan(&date); err != nil {
				return err
			}
			dates = append(dates, date)
		}
		return rows.Err()
	})
	return dates, err
}

// peek opens the database read only without the lock, checks its schema and
// runs read in one read transaction, so everything read comes from the same
// saved state. A missing database reads as empty, and nothing is created.
func peek(ctx context.Context, path string, read func(*sql.Tx) error) (err error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	} else if errors.Is(err, os.ErrNotExist) {
		return nil
	} else {
		return err
	}
	if _, err := os.Stat(absolute + repairSuffix); err == nil {
		return ErrIncompleteRepair
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := plainFile(absolute + suffix); err != nil {
			return err
		}
	}
	db, err := connect(ctx, absolute, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if err := checkSchema(ctx, db); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return classify(err)
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	if err := read(tx); err != nil {
		return classify(err)
	}
	return nil
}
