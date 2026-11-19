package planner

import (
	"context"
	"errors"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// Operation names what a person asked for.
type Operation string

const (
	Add    Operation = "add"
	Edit   Operation = "edit"
	Delete Operation = "delete"
	Start  Operation = "start"
	Pause  Operation = "pause"
	Resume Operation = "resume"
	Finish Operation = "finish"
	Skip   Operation = "skip"
	Unskip Operation = "unskip"
	Reopen Operation = "reopen"
	Carry  Operation = "carry"
)

// Intent is one explicit request against one day.
type Intent struct {
	Operation Operation
	Selection Selection

	// ItemID and Revision name the item a change is based on. Add ignores
	// them; IntendedID may give the new item its ID in advance, so an
	// unconfirmed add can be found again by Reconcile.
	ItemID     domain.ID
	Revision   Revision
	IntendedID domain.ID

	// Title and Duration are the text a person typed. Add needs both. Edit
	// changes whichever is not nil.
	Title    *string
	Duration *string

	// Sources are the items Carry copies onto the selected day, in order.
	Sources []domain.ID
}

// ErrPastDay means a past date was asked to change.
var ErrPastDay = errors.New("past days can only be looked at")

// ErrDurationExceedsDay means a Duration is longer than the time left on its
// day. It also counts as an invalid Duration, so it is shown at that field.
var ErrDurationExceedsDay = errors.New("the duration is longer than the time left on this day")

// Do carries out an intent and reports exactly what happened.
func (c Client) Do(ctx context.Context, intent Intent) (Result, error) {
	s, err := c.open(ctx, intent.Selection)
	if err != nil {
		return Result{}, err
	}
	result, err := s.do(ctx, intent)
	return result, errors.Join(err, s.db.Close())
}

func (s *session) do(ctx context.Context, intent Intent) (Result, error) {
	switch intent.Operation {
	case Add:
		return s.add(ctx, intent)
	case Edit:
		return s.edit(ctx, intent)
	case Delete:
		return s.remove(ctx, intent)
	case Start, Pause, Resume, Finish, Skip, Unskip, Reopen:
		return s.track(ctx, intent)
	case Carry:
		return s.carry(ctx, intent)
	}
	return Result{}, InputError(nil, "Unknown operation "+string(intent.Operation), "")
}

// requirePlanning refuses changes to the plan of a past or closed day. What
// counts as today comes from this computer's timezone, so choosing another
// timezone cannot reopen the past.
func (s *session) requirePlanning() error {
	today := s.now.In(s.zone).Format("2006-01-02")
	if s.day.Date < today {
		return Refusal(ErrPastDay, "Past days can only be looked at", "Plan new work for today or a later date")
	}
	if _, end, err := s.day.Bounds(); err != nil {
		return err
	} else if !s.now.Before(end) {
		return Refusal(ErrPastDay, "This day has already ended", "Plan new work for today or a later date")
	}
	return nil
}

func (s *session) add(ctx context.Context, intent Intent) (Result, error) {
	if err := s.requirePlanning(); err != nil {
		return Result{}, err
	}
	if intent.Title == nil || intent.Duration == nil {
		return Result{}, InputError(nil, "A new item needs a title and a duration", "")
	}
	minutes, err := ParseDuration(*intent.Duration)
	if err != nil {
		return Result{}, err
	}
	if err := s.fits(ctx, minutes, ""); err != nil {
		return Result{}, err
	}
	day := s.day
	var newDay *domain.Day
	if day.ID == "" {
		if day, err = domain.NewDay(day.Date, day.Timezone, s.now); err != nil {
			return Result{}, err
		}
		newDay = &day
	}
	item, err := domain.NewItem(day.ID, *intent.Title, minutes, s.now)
	if err != nil {
		return Result{}, err
	}
	if intent.IntendedID != "" {
		item.ID = intent.IntendedID
	}
	event := domain.Record(nil, item, s.now, s.now, domain.ReasonUser)
	s.day = day
	return s.persist(ctx, item, event, nil, func() error { return s.db.Create(ctx, newDay, store.Change{Item: item, Event: event}) })
}

func (s *session) edit(ctx context.Context, intent Intent) (Result, error) {
	if err := s.requirePlanning(); err != nil {
		return Result{}, err
	}
	item, err := s.current(ctx, intent.ItemID, intent.Revision)
	if err != nil {
		return Result{}, err
	}
	before := item.Clone()
	title, minutes := item.Title, item.DurationMinutes
	if intent.Title == nil && intent.Duration == nil {
		return Result{}, InputError(nil, "Say what to change", "Give a new title, a new duration, or both")
	}
	if intent.Title != nil {
		title = *intent.Title
	}
	if intent.Duration != nil {
		if minutes, err = ParseDuration(*intent.Duration); err != nil {
			return Result{}, err
		}
		// Only a longer Duration is checked, so an item on an already full
		// day can always be shortened or renamed.
		if minutes > item.DurationMinutes {
			if err := s.fits(ctx, minutes, item.ID); err != nil {
				return Result{}, err
			}
		}
	}
	if err := item.Edit(title, minutes, s.now); err != nil {
		return Result{}, err
	}
	event := domain.Record(&before, item, s.now, s.now, domain.ReasonUser)
	return s.persist(ctx, item, event, nil, func() error { return s.db.Save(ctx, store.Change{Item: item, Event: event}) })
}

func (s *session) remove(ctx context.Context, intent Intent) (Result, error) {
	if err := s.requirePlanning(); err != nil {
		return Result{}, err
	}
	item, err := s.current(ctx, intent.ItemID, intent.Revision)
	if err != nil {
		return Result{}, err
	}
	before := item.Clone()
	if err := item.Archive(s.now); err != nil {
		return Result{}, err
	}
	event := domain.Record(&before, item, s.now, s.now, domain.ReasonUser)
	return s.persist(ctx, item, event, nil, func() error { return s.db.Save(ctx, store.Change{Item: item, Event: event}) })
}

// track performs a status action. Starting, resuming or reopening an item
// pauses whatever else was running, in the same save and at the same instant.
func (s *session) track(ctx context.Context, intent Intent) (Result, error) {
	item, err := s.current(ctx, intent.ItemID, intent.Revision)
	if err != nil {
		return Result{}, err
	}
	before := item.Clone()
	if err := item.Apply(domain.Action(intent.Operation), s.now, s.day); err != nil {
		return Result{}, err
	}
	reason := domain.ReasonUser
	if intent.Operation == Reopen {
		reason = domain.ReasonReopen
	}
	var changes []store.Change
	var paused []domain.ID
	if item.Status == domain.InProgress {
		others, err := s.db.Items(ctx, s.day.ID)
		if err != nil {
			return Result{}, err
		}
		for _, other := range others {
			if other.ID == item.ID || other.Status != domain.InProgress {
				continue
			}
			prior := other.Clone()
			if err := other.Apply(domain.Pause, s.now, s.day); err != nil {
				return Result{}, err
			}
			changes = append(changes, store.Change{Item: other, Event: domain.Record(&prior, other, s.now, s.now, domain.ReasonAutoPause)})
			paused = append(paused, other.ID)
		}
	}
	event := domain.Record(&before, item, s.now, s.now, reason)
	changes = append(changes, store.Change{Item: item, Event: event})
	return s.persist(ctx, item, event, paused, func() error { return s.db.Save(ctx, changes...) })
}

// ParseDuration reads typed Duration text, refusing it with friendly
// feedback when it is not a positive number of whole minutes.
func ParseDuration(text string) (int64, error) {
	minutes, err := domain.ParseDuration(text)
	if err != nil {
		return 0, InputError(err, "Invalid duration "+quote(text), "Use whole minutes or hours, like 30m, 2h or 1h30m")
	}
	return minutes, nil
}

// remaining is the time a Duration may use on the day on screen. The item
// being edited does not count against itself.
func (s *session) remaining(ctx context.Context, editing domain.ID) (int64, error) {
	var others []domain.Reservation
	if s.day.ID != "" {
		items, err := s.db.Items(ctx, s.day.ID)
		if err != nil {
			return 0, err
		}
		history, err := s.db.DayEvents(ctx, s.day.ID)
		if err != nil {
			return 0, err
		}
		for _, item := range items {
			if item.ID == editing || !item.Status.Unfinished() {
				continue
			}
			metrics, err := domain.Measure(s.day, item, history[item.ID], s.now)
			if err != nil {
				return 0, err
			}
			others = append(others, domain.Reservation{Status: item.Status, DurationMinutes: item.DurationMinutes, WorkNS: metrics.WorkNS})
		}
	}
	return domain.RemainingMinutes(s.day, s.now, others)
}

// fits refuses a Duration longer than the time left, saying how much is left.
func (s *session) fits(ctx context.Context, minutes int64, editing domain.ID) error {
	left, err := s.remaining(ctx, editing)
	if err != nil || minutes <= left {
		return err
	}
	cause := errors.Join(ErrDurationExceedsDay, domain.ErrInvalidDuration)
	if left == 0 {
		return Refusal(cause, "No time left on this day", "Plan this on another day, or free some time by skipping or deleting an item")
	}
	text, _ := domain.FormatDuration(left)
	return Refusal(cause, "Only "+text+" left on this day", "Enter "+text+" or less, or plan this on another day")
}
