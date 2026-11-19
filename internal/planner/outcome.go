package planner

import (
	"context"
	"errors"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// Outcome says whether a change happened.
type Outcome string

const (
	// Refused means nothing was saved, for a known reason.
	Refused Outcome = "refused"
	// Confirmed means the change is on disk.
	Confirmed Outcome = "confirmed"
	// Unconfirmed means the save failed in a way that leaves it unknown
	// whether it reached the disk. Do not retry it blindly: Reconcile first.
	Unconfirmed Outcome = "unconfirmed"
)

// Result describes one change. After a confirmed save it carries the saved
// item, its fresh history and revision, and its metrics at the same instant.
type Result struct {
	Outcome Outcome
	At      time.Time
	Day     domain.Day
	Item    domain.Item
	Metrics domain.Metrics
	// Carried are every item a carry created, in order. Item is the first.
	Carried    []domain.Item
	AutoPaused []domain.ID
	History    map[domain.ID][]domain.Event
	Revisions  map[domain.ID]Revision

	// Intended and BeforeSeq let Reconcile recognise this change in the
	// saved history later.
	Intended  domain.Item
	Reason    domain.Reason
	BeforeSeq int64
}

// persist runs one write and turns its outcome into a Result. A confirmed
// write stays confirmed even if reading it back afterwards fails.
func (s *session) persist(ctx context.Context, item domain.Item, event domain.Event, paused []domain.ID, write func() error) (Result, error) {
	result := Result{Outcome: Refused, At: s.now, Day: s.day, Item: item, AutoPaused: paused, Intended: item.Clone(), Reason: event.Reason}
	if event.Before != nil {
		events, err := s.db.Events(ctx, item.ID)
		if err != nil {
			return result, err
		}
		if len(events) > 0 {
			result.BeforeSeq = events[len(events)-1].Seq
		}
	}
	if err := write(); err != nil {
		if !refusal(err) {
			result.Outcome = Unconfirmed
		}
		return result, err
	}
	result.Outcome = Confirmed
	return result, s.reload(ctx, &result)
}

// refusal reports whether a failed write certainly changed nothing.
func refusal(err error) bool {
	for _, known := range []error{ErrNotCarryable, store.ErrStale, store.ErrInvalidSave, store.ErrNotFound, store.ErrLocked, domain.ErrIllegalTransition, domain.ErrInvalidEvidence, domain.ErrAlreadyRunning, domain.ErrDayClosed, domain.ErrInvalidTitle, domain.ErrInvalidDuration} {
		if errors.Is(err, known) {
			return true
		}
	}
	return false
}

// reload fills a confirmed result with what is now saved.
func (s *session) reload(ctx context.Context, result *Result) error {
	saved, err := s.db.Item(ctx, result.Item.ID)
	if err != nil {
		return err
	}
	result.Item = saved
	result.History = map[domain.ID][]domain.Event{}
	result.Revisions = map[domain.ID]Revision{}
	for _, id := range append([]domain.ID{saved.ID}, result.AutoPaused...) {
		item, err := s.db.Item(ctx, id)
		if err != nil {
			return err
		}
		events, err := s.db.Events(ctx, id)
		if err != nil {
			return err
		}
		if result.Revisions[id], err = revisionOf(s.day, item, events); err != nil {
			return err
		}
		result.History[id] = events
	}
	result.Metrics, err = domain.Measure(s.day, saved, result.History[saved.ID], s.now)
	return err
}

// ErrStillUnknown means the saved history neither shows the change nor rules
// it out. Look at the item before trying again.
var ErrStillUnknown = errors.New("it is still unknown whether the change was saved")

// Reconcile looks for an unconfirmed change in the saved history and returns
// it as confirmed or refused. A later automatic change, such as the day
// closing at midnight, does not hide that the original change happened.
func (c Client) Reconcile(ctx context.Context, pending Result) (result Result, err error) {
	s, err := c.open(ctx, Selection{Date: pending.Day.Date, Timezone: pending.Day.Timezone})
	if err != nil {
		return pending, err
	}
	defer func() { err = errors.Join(err, s.db.Close()) }()
	result = pending
	item, err := s.db.Item(ctx, pending.Intended.ID)
	if errors.Is(err, store.ErrNotFound) {
		result.Outcome = Refused
		return result, nil
	}
	if err != nil {
		return result, err
	}
	events, err := s.db.Events(ctx, item.ID)
	if err != nil {
		return result, err
	}
	for _, e := range events {
		if e.Seq > pending.BeforeSeq && e.OccurredAt.Equal(pending.At) && e.Reason == pending.Reason && domain.Same(e.After, pending.Intended) {
			result.Outcome = Confirmed
			result.Item = item
			return result, s.reload(ctx, &result)
		}
	}
	if len(events) > 0 && events[len(events)-1].Seq == pending.BeforeSeq {
		result.Outcome = Refused
		return result, nil
	}
	return result, ErrStillUnknown
}
