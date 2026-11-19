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
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Glance{}, err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	} else if errors.Is(err, os.ErrNotExist) {
		return Glance{}, nil
	} else {
		return Glance{}, err
	}
	if _, err := os.Stat(absolute + repairSuffix); err == nil {
		return Glance{}, ErrIncompleteRepair
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := plainFile(absolute + suffix); err != nil {
			return Glance{}, err
		}
	}
	db, err := connect(ctx, absolute, true)
	if err != nil {
		return Glance{}, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if err := checkSchema(ctx, db); err != nil {
		return Glance{}, err
	}
	// One read transaction, so the day, the item and its history all come
	// from the same saved state.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Glance{}, classify(err)
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	day, err := readDay(ctx, tx, "date", date)
	if errors.Is(err, ErrNotFound) {
		return Glance{}, nil
	}
	if err != nil {
		return Glance{}, classify(err)
	}
	glance := Glance{Day: day}
	item, err := scanItem(tx.QueryRowContext(ctx, "SELECT "+itemColumns+" FROM items WHERE day_id=? AND archived=0 AND status=?", day.ID, domain.InProgress))
	if errors.Is(err, ErrNotFound) {
		return glance, nil
	}
	if err != nil {
		return Glance{}, classify(err)
	}
	events, err := readEvents(ctx, tx, "item_id=?", item.ID)
	if err != nil {
		return Glance{}, classify(err)
	}
	if len(events) == 0 {
		return Glance{}, fmt.Errorf("%w: item %s has no history", ErrCorrupt, item.ID)
	}
	glance.Item, glance.Events, glance.Running = item, events, true
	return glance, nil
}
