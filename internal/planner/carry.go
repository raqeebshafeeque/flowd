package planner

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// ErrNotCarryable means an item cannot be carried: it is not incomplete or
// skipped, it already has a copy, or its day is outside the window.
var ErrNotCarryable = errors.New("the item cannot be carried")

// Leftover is an item that can be carried, and the date it is on.
type Leftover struct {
	Date string      `json:"date"`
	Item domain.Item `json:"item"`
}

// window is the range of dates items can be carried from onto date: the
// seven days before it, and never today or later, because only past days
// are finished.
func (s *session) window(date string) (first, last string) {
	d, _ := time.Parse("2006-01-02", date)
	first = d.AddDate(0, 0, -domain.CarryDays).Format("2006-01-02")
	last = d.AddDate(0, 0, -1).Format("2006-01-02")
	if yesterday := s.now.In(s.zone).AddDate(0, 0, -1).Format("2006-01-02"); last > yesterday {
		last = yesterday
	}
	return first, last
}

// leftovers lists what can be carried onto the day on screen, newest day
// first and in each day's own order.
func (s *session) leftovers(ctx context.Context) ([]Leftover, error) {
	first, last := s.window(s.day.Date)
	found, err := s.db.Leftovers(ctx, first, last)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(found, func(a, b store.Leftover) int {
		if a.Day.Date != b.Day.Date {
			return -compareText(a.Day.Date, b.Day.Date)
		}
		return domain.Compare(a.Item, b.Item)
	})
	out := make([]Leftover, 0, len(found))
	for _, f := range found {
		out = append(out, Leftover{f.Day.Date, f.Item})
	}
	return out, nil
}

func compareText(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Leftovers lists the items that can be carried onto the selected day:
// incomplete or skipped items from the seven days before it that have no
// copy yet.
func (c Client) Leftovers(ctx context.Context, selection Selection) (items []Leftover, err error) {
	s, err := c.open(ctx, selection)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, s.db.Close()) }()
	return s.leftovers(ctx)
}

// carry copies the sources onto the day on screen as new todo items with
// the same title and duration, all in one save. If they do not all fit in
// what is left of the day, nothing is carried.
func (s *session) carry(ctx context.Context, intent Intent) (Result, error) {
	if err := s.requirePlanning(); err != nil {
		return Result{}, err
	}
	if len(intent.Sources) == 0 {
		return Result{}, InputError(nil, "Pick at least one item to carry", "Find them with flowd carry at a terminal, or use --all")
	}
	available, err := s.leftovers(ctx)
	if err != nil {
		return Result{}, err
	}
	byID := map[domain.ID]Leftover{}
	for _, l := range available {
		byID[l.Item.ID] = l
	}
	var sources []domain.Item
	var total int64
	seen := map[domain.ID]bool{}
	for _, id := range intent.Sources {
		if seen[id] {
			continue
		}
		seen[id] = true
		l, ok := byID[id]
		if !ok {
			return Result{}, s.whyNot(ctx, id)
		}
		sources = append(sources, l.Item)
		total += l.Item.DurationMinutes
	}
	if left, err := s.remaining(ctx, ""); err != nil {
		return Result{}, err
	} else if total > left {
		need, _ := domain.FormatDuration(total)
		have := "no time"
		if left > 0 {
			have, _ = domain.FormatDuration(left)
			have = "only " + have
		}
		picked := "The picked item needs "
		if len(sources) > 1 {
			picked = fmt.Sprintf("The %d picked items need ", len(sources))
		}
		return Result{}, Refusal(errors.Join(ErrDurationExceedsDay, domain.ErrInvalidDuration),
			picked+need+", and "+have+" is left on "+s.day.Date, "Pick fewer items, or carry them onto another day with --date")
	}
	day := s.day
	var newDay *domain.Day
	if day.ID == "" {
		if day, err = domain.NewDay(day.Date, day.Timezone, s.now); err != nil {
			return Result{}, err
		}
		newDay = &day
	}
	var changes []store.Change
	var copies []domain.Item
	for _, source := range sources {
		copied, err := source.CarryTo(day.ID, s.now)
		if err != nil {
			return Result{}, err
		}
		copies = append(copies, copied)
		changes = append(changes, store.Change{Item: copied, Event: domain.Record(nil, copied, s.now, s.now, domain.ReasonUser)})
	}
	s.day = day
	result, err := s.persist(ctx, copies[0], changes[0].Event, nil, func() error { return s.db.Create(ctx, newDay, changes...) })
	if result.Outcome == Confirmed {
		for i, c := range copies {
			if saved, err := s.db.Item(ctx, c.ID); err == nil {
				copies[i] = saved
			}
		}
	}
	result.Carried = copies
	return result, err
}

// whyNot explains why an item asked for cannot be carried.
func (s *session) whyNot(ctx context.Context, id domain.ID) error {
	item, err := s.db.Item(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return Refusal(store.ErrNotFound, "No item "+string(id), "Find what can be carried with flowd carry at a terminal")
	}
	if err != nil {
		return err
	}
	switch {
	case !item.Carryable():
		return Refusal(errors.Join(ErrNotCarryable, domain.ErrIllegalTransition), "Only incomplete or skipped items can be carried", "Start or finish it on its own day instead")
	}
	// It is carryable but was not offered, so either its day is outside the
	// window or it has a copy already.
	first, last := s.window(s.day.Date)
	owner, err := s.db.DayByID(ctx, item.DayID)
	if err != nil {
		return err
	}
	if owner.Date < first || owner.Date > last {
		return Refusal(ErrNotCarryable, fmt.Sprintf("Only items from the %d days before %s can be carried", domain.CarryDays, s.day.Date), "Plan it again with flowd add")
	}
	return Refusal(ErrNotCarryable, "That item was carried already", "Its copy is still planned. Delete the copy to carry it again")
}
