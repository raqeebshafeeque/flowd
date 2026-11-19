package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

const (
	dayColumns   = "id,date,timezone,created_at,updated_at"
	itemColumns  = "id,day_id,title,status,duration_minutes,started_at,finished_at,closed_at,archived,created_at,updated_at,carried_from"
	eventColumns = "seq,item_id,from_status,to_status,occurred_at,recorded_at,reason,before_json,after_json"
)

// querier is what both *sql.DB and *sql.Tx can do.
type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// DayByDate returns the saved day for a date, or ErrNotFound.
func (s *Store) DayByDate(ctx context.Context, date string) (domain.Day, error) {
	return readDay(ctx, s.db, "date", date)
}

// Items returns the visible items of a day in the order set for it.
func (s *Store) Items(ctx context.Context, dayID domain.ID) ([]domain.Item, error) {
	items, err := readItems(ctx, s.db, dayID)
	return items, classify(err)
}

// Item returns one item, including a deleted one, or ErrNotFound.
func (s *Store) Item(ctx context.Context, id domain.ID) (domain.Item, error) {
	item, err := scanItem(s.db.QueryRowContext(ctx, "SELECT "+itemColumns+" FROM items WHERE id=?", id))
	return item, classify(err)
}

// Events returns the history of one item in order.
func (s *Store) Events(ctx context.Context, itemID domain.ID) ([]domain.Event, error) {
	events, err := readEvents(ctx, s.db, "item_id=?", itemID)
	return events, classify(err)
}

// DayEvents returns the history of every visible item of a day in one query.
func (s *Store) DayEvents(ctx context.Context, dayID domain.ID) (map[domain.ID][]domain.Event, error) {
	events, err := readEvents(ctx, s.db, "item_id IN (SELECT id FROM items WHERE day_id=? AND archived=0)", dayID)
	if err != nil {
		return nil, classify(err)
	}
	byItem := map[domain.ID][]domain.Event{}
	for _, e := range events {
		byItem[e.ItemID] = append(byItem[e.ItemID], e)
	}
	return byItem, nil
}

// Create saves new items and their creation events, all in one
// transaction, so either every one is saved or none is. They all belong to
// one day. When newDay is not nil the day is created in the same
// transaction, so a day never exists without the items that caused it.
func (s *Store) Create(ctx context.Context, newDay *domain.Day, items ...Change) error {
	if len(items) == 0 {
		return ErrInvalidSave
	}
	for _, c := range items {
		if c.Event.Before != nil || c.Event.Seq != 0 || !domain.Same(c.Item, c.Event.After) || c.Item.DayID != items[0].Item.DayID || (newDay != nil && newDay.ID != c.Item.DayID) {
			return ErrInvalidSave
		}
	}
	return s.transact(ctx, func(tx *sql.Tx) error {
		if newDay != nil {
			if _, err := tx.ExecContext(ctx, "INSERT INTO days("+dayColumns+") VALUES(?,?,?,?,?)", newDay.ID, newDay.Date, newDay.Timezone, instant(newDay.CreatedAt), nullable(newDay.UpdatedAt)); err != nil {
				return err
			}
		}
		for _, c := range items {
			// A new item goes to the end of its day's list.
			values := append(itemValues(c.Item), carriedFrom(c.Item), c.Item.DayID)
			if _, err := tx.ExecContext(ctx, "INSERT INTO items("+itemColumns+",position) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,(SELECT COALESCE(MAX(position), 0) + 1 FROM items WHERE day_id=? AND archived=0))", values...); err != nil {
				return err
			}
			if err := record(ctx, tx, c.Item.DayID, c.Event); err != nil {
				return err
			}
		}
		return nil
	})
}

// Change is a saved item and the event that explains how it got there.
type Change struct {
	Item  domain.Item
	Event domain.Event
}

// Save stores changes to existing items in one transaction. Each event's
// before state has to match what is saved, so a change based on an old read
// fails with ErrStale instead of overwriting newer work.
func (s *Store) Save(ctx context.Context, changes ...Change) error {
	return s.transact(ctx, func(tx *sql.Tx) error { return save(ctx, tx, changes) })
}

func save(ctx context.Context, tx *sql.Tx, changes []Change) error {
	seen := map[domain.ID]bool{}
	for _, c := range changes {
		if seen[c.Item.ID] || c.Event.Before == nil || c.Event.Seq != 0 || !domain.Same(c.Item, c.Event.After) {
			return ErrInvalidSave
		}
		seen[c.Item.ID] = true
		saved, err := scanItem(tx.QueryRowContext(ctx, "SELECT "+itemColumns+" FROM items WHERE id=?", c.Item.ID))
		if err != nil {
			return err
		}
		if !domain.Same(saved, *c.Event.Before) {
			return ErrStale
		}
		values := itemValues(c.Item)
		if _, err := tx.ExecContext(ctx, "UPDATE items SET day_id=?,title=?,status=?,duration_minutes=?,started_at=?,finished_at=?,closed_at=?,archived=?,created_at=?,updated_at=? WHERE id=?", append(values[1:], c.Item.ID)...); err != nil {
			return err
		}
		if err := record(ctx, tx, c.Item.DayID, c.Event); err != nil {
			return err
		}
		// A deleted item gives up its place, and the rest close the gap.
		if c.Item.Archived && !c.Event.Before.Archived {
			if _, err := tx.ExecContext(ctx, "UPDATE items SET position = 0 WHERE id=?", c.Item.ID); err != nil {
				return err
			}
			if err := renumber(ctx, tx, c.Item.DayID, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// ErrOrderChanged means a new order did not name exactly the visible items
// of the day, because something was added, deleted or named twice.
var ErrOrderChanged = errors.New("the day changed while it was being reordered")

// SetOrder saves a day's order: order names every visible item of the day
// exactly once, in the order they are to be listed. Anything else is refused
// with ErrOrderChanged and nothing is saved. The order is how the list is
// laid out, not part of any item's history, so it records no events.
func (s *Store) SetOrder(ctx context.Context, dayID domain.ID, order []domain.ID) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		current, err := visibleIDs(ctx, tx, dayID)
		if err != nil {
			return err
		}
		if len(order) != len(current) {
			return ErrOrderChanged
		}
		named := map[domain.ID]bool{}
		for _, id := range current {
			named[id] = false
		}
		for _, id := range order {
			if done, ok := named[id]; !ok || done {
				return ErrOrderChanged
			}
			named[id] = true
		}
		return renumber(ctx, tx, dayID, order)
	})
}

// visibleIDs lists a day's visible items in their current order.
func visibleIDs(ctx context.Context, tx *sql.Tx, dayID domain.ID) ([]domain.ID, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM items WHERE day_id=? AND archived=0 ORDER BY position, id", dayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []domain.ID
	for rows.Next() {
		var id domain.ID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// renumber gives a day's visible items the positions 1, 2, 3 and so on, in
// the given order, or in their current order when order is nil. No two
// visible items of a day may share a position, and SQLite checks that after
// every row, so it is done in two passes: first every item takes its new
// position as a negative number, then they all turn positive. No two rows
// share a number at any moment.
func renumber(ctx context.Context, tx *sql.Tx, dayID domain.ID, order []domain.ID) error {
	if order == nil {
		var err error
		if order, err = visibleIDs(ctx, tx, dayID); err != nil {
			return err
		}
	}
	for i, id := range order {
		if _, err := tx.ExecContext(ctx, "UPDATE items SET position=? WHERE id=?", -(i + 1), id); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, "UPDATE items SET position = -position WHERE day_id=? AND archived=0 AND position < 0", dayID)
	return err
}

// record appends an event, then checks the whole day: the event fits its day,
// the clock did not go backwards, the items agree with each other and every
// item's history still adds up.
func record(ctx context.Context, tx *sql.Tx, dayID domain.ID, e domain.Event) error {
	day, err := readDay(ctx, tx, "id", string(dayID))
	if err != nil {
		return err
	}
	if err := e.ValidateFor(day); err != nil {
		return err
	}
	var latest sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT max(recorded_at) FROM item_events WHERE item_id IN (SELECT id FROM items WHERE day_id=?)", dayID).Scan(&latest); err != nil {
		return err
	}
	if latest.Valid && instant(e.RecordedAt) < latest.String {
		return fmt.Errorf("%w: the clock is earlier than the day's last change", domain.ErrInvalidEvidence)
	}
	before, err := encode(e.Before)
	if err != nil {
		return err
	}
	after, err := json.Marshal(e.After)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO item_events(item_id,from_status,to_status,occurred_at,recorded_at,reason,before_json,after_json) VALUES(?,?,?,?,?,?,?,?)",
		e.ItemID, e.FromStatus, e.ToStatus, instant(e.OccurredAt), instant(e.RecordedAt), e.Reason, before, string(after)); err != nil {
		return err
	}
	return checkDay(ctx, tx, day)
}

// checkDay validates a whole day inside the transaction that changed it.
func checkDay(ctx context.Context, tx *sql.Tx, day domain.Day) error {
	items, err := readItems(ctx, tx, day.ID)
	if err != nil {
		return err
	}
	if err := domain.CheckDay(items); err != nil {
		return err
	}
	for _, item := range items {
		events, err := readEvents(ctx, tx, "item_id=?", item.ID)
		if err != nil {
			return err
		}
		observed := item.CreatedAt
		if item.UpdatedAt != nil {
			observed = *item.UpdatedAt
		}
		if _, err := domain.Measure(day, item, events, observed); err != nil {
			return err
		}
	}
	return nil
}

func readDay(ctx context.Context, q querier, column, value string) (domain.Day, error) {
	var day domain.Day
	var created string
	var updated sql.NullString
	err := q.QueryRowContext(ctx, "SELECT "+dayColumns+" FROM days WHERE "+column+"=?", value).Scan(&day.ID, &day.Date, &day.Timezone, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Day{}, ErrNotFound
	}
	if err != nil {
		return domain.Day{}, classify(err)
	}
	if day.CreatedAt, err = parseInstant(created); err != nil {
		return domain.Day{}, err
	}
	day.UpdatedAt, err = parseNullable(updated)
	return day, err
}

func readItems(ctx context.Context, q querier, dayID domain.ID) ([]domain.Item, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+itemColumns+" FROM items WHERE day_id=? AND archived=0 ORDER BY position, id", dayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func scanItem(row interface{ Scan(...any) error }) (domain.Item, error) {
	var it domain.Item
	var created string
	var started, finished, closed, updated, carried sql.NullString
	if err := row.Scan(&it.ID, &it.DayID, &it.Title, &it.Status, &it.DurationMinutes, &started, &finished, &closed, &it.Archived, &created, &updated, &carried); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Item{}, ErrNotFound
		}
		return domain.Item{}, err
	}
	// A title saved by an older version may hold hidden characters. It is
	// shown with visible <U+XXXX> placeholders, never as it was.
	it.Title = domain.SanitizeText(it.Title)
	if carried.Valid {
		from := domain.ID(carried.String)
		it.CarriedFrom = &from
	}
	var err error
	if it.CreatedAt, err = parseInstant(created); err != nil {
		return domain.Item{}, err
	}
	for _, f := range []struct {
		text sql.NullString
		into **time.Time
	}{{started, &it.StartedAt}, {finished, &it.FinishedAt}, {closed, &it.ClosedAt}, {updated, &it.UpdatedAt}} {
		if *f.into, err = parseNullable(f.text); err != nil {
			return domain.Item{}, err
		}
	}
	return it, nil
}

func readEvents(ctx context.Context, q querier, where string, arg any) ([]domain.Event, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+eventColumns+" FROM item_events WHERE "+where+" ORDER BY seq", arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []domain.Event{}
	for rows.Next() {
		var e domain.Event
		var occurred, recorded, after string
		var before sql.NullString
		if err := rows.Scan(&e.Seq, &e.ItemID, &e.FromStatus, &e.ToStatus, &occurred, &recorded, &e.Reason, &before, &after); err != nil {
			return nil, err
		}
		if e.OccurredAt, err = parseInstant(occurred); err != nil {
			return nil, err
		}
		if e.RecordedAt, err = parseInstant(recorded); err != nil {
			return nil, err
		}
		if before.Valid {
			var b domain.Item
			if err := decode(before.String, &b); err != nil {
				return nil, err
			}
			e.Before = &b
		}
		if err := decode(after, &e.After); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// carriedFrom is the stored form of where an item was copied from. It is
// only written when the item is created, because it never changes.
func carriedFrom(it domain.Item) any {
	if it.CarriedFrom == nil {
		return nil
	}
	return string(*it.CarriedFrom)
}

func itemValues(it domain.Item) []any {
	return []any{it.ID, it.DayID, it.Title, it.Status, it.DurationMinutes, nullable(it.StartedAt), nullable(it.FinishedAt), nullable(it.ClosedAt), it.Archived, instant(it.CreatedAt), nullable(it.UpdatedAt)}
}

// instant is how a time is stored: UTC, whole seconds, RFC 3339. Text in this
// form sorts in time order.
func instant(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func nullable(t *time.Time) any {
	if t == nil {
		return nil
	}
	return instant(*t)
}

// parseInstant reads a stored time. Anything that is not a whole second in
// UTC was not written by flowd, so the file is treated as damaged.
func parseInstant(text string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, text)
	if err != nil || instant(t) != text {
		return time.Time{}, fmt.Errorf("%w: stored time %q", ErrCorrupt, text)
	}
	return t.UTC(), nil
}

func parseNullable(text sql.NullString) (*time.Time, error) {
	if !text.Valid {
		return nil, nil
	}
	t, err := parseInstant(text.String)
	return &t, err
}

func encode(item *domain.Item) (any, error) {
	if item == nil {
		return nil, nil
	}
	raw, err := json.Marshal(item)
	return string(raw), err
}

// decode reads a stored item copy, refusing unknown fields.
func decode(text string, into *domain.Item) error {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("%w: stored item copy: %w", ErrCorrupt, err)
	}
	into.Title = domain.SanitizeText(into.Title)
	for _, t := range []*time.Time{&into.CreatedAt, into.StartedAt, into.FinishedAt, into.ClosedAt, into.UpdatedAt} {
		if t != nil && !t.Equal(t.Truncate(time.Second)) {
			return fmt.Errorf("%w: stored item copy has a fractional time", ErrCorrupt)
		}
	}
	return nil
}

// Leftover is an item that can be carried, with the day it is on.
type Leftover struct {
	Day  domain.Day
	Item domain.Item
}

// Leftovers lists the items on days from first to last, both included,
// that can be carried: incomplete or skipped, not deleted, and without a
// copy that is still there. A copy that was deleted frees its item again.
// Newer days come first.
func (s *Store) Leftovers(ctx context.Context, first, last string) ([]Leftover, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.id, i.id FROM items i JOIN days d ON d.id = i.day_id
 WHERE d.date >= ? AND d.date <= ? AND i.archived = 0 AND i.status IN ('incomplete', 'skipped')
 AND NOT EXISTS (SELECT 1 FROM items c WHERE c.carried_from = i.id AND c.archived = 0)
 ORDER BY d.date DESC, i.position, i.id`, first, last)
	if err != nil {
		return nil, classify(err)
	}
	type pair struct{ day, item domain.ID }
	var found []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.day, &p.item); err != nil {
			rows.Close()
			return nil, classify(err)
		}
		found = append(found, p)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, classify(err)
	}
	days := map[domain.ID]domain.Day{}
	var out []Leftover
	for _, p := range found {
		day, ok := days[p.day]
		if !ok {
			if day, err = readDay(ctx, s.db, "id", string(p.day)); err != nil {
				return nil, classify(err)
			}
			days[p.day] = day
		}
		item, err := s.Item(ctx, p.item)
		if err != nil {
			return nil, err
		}
		out = append(out, Leftover{day, item})
	}
	return out, nil
}

// DayByID returns the saved day with that ID, or ErrNotFound.
func (s *Store) DayByID(ctx context.Context, id domain.ID) (domain.Day, error) {
	return readDay(ctx, s.db, "id", string(id))
}
