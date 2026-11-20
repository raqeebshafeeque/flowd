package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Reason says why an event happened.
type Reason string

const (
	ReasonUser      Reason = "user"       // someone asked for it
	ReasonAutoPause Reason = "auto_pause" // another item started
	ReasonReopen    Reason = "reopen"     // a done item was reopened
	ReasonDayEnd    Reason = "day_end"    // the day closed at midnight
)

// Event is one saved change to an item. It keeps a full copy of the item
// before and after, so history can be checked and replayed. Before is empty
// only for the event that created the item. Events are never edited.
type Event struct {
	Seq        int64     `json:"seq"`
	ItemID     ID        `json:"item_id"`
	FromStatus Status    `json:"from_status"`
	ToStatus   Status    `json:"to_status"`
	OccurredAt time.Time `json:"occurred_at"`
	RecordedAt time.Time `json:"recorded_at"`
	Reason     Reason    `json:"reason"`
	Before     *Item     `json:"before"`
	After      Item      `json:"after"`
}

// Record builds the event for a change from before to after. Use nil for
// before when the item is new.
func Record(before *Item, after Item, occurred, recorded time.Time, reason Reason) Event {
	e := Event{ItemID: after.ID, FromStatus: after.Status, ToStatus: after.Status, OccurredAt: occurred.UTC(), RecordedAt: recorded.UTC(), Reason: reason, After: after.Clone()}
	if before != nil {
		b := before.Clone()
		e.Before = &b
		e.FromStatus = b.Status
	}
	return e
}

// Same reports whether two items hold exactly the same saved values.
func Same(a, b Item) bool {
	x, _ := json.Marshal(a.Clone())
	y, _ := json.Marshal(b.Clone())
	return string(x) == string(y)
}

// Validate replays the change and refuses any event whose after state could
// not have come from its before state through a real action.
func (e Event) Validate() error {
	bad := func(why string) error { return fmt.Errorf("%w: event for %s %s", ErrInvalidEvidence, e.ItemID, why) }
	if e.ItemID != e.After.ID || !e.FromStatus.Valid() || e.ToStatus != e.After.Status || !ValidInstant(e.OccurredAt) || !ValidInstant(e.RecordedAt) || e.RecordedAt.Before(e.OccurredAt) {
		return bad("is not consistent")
	}
	if err := e.After.Check(); err != nil {
		return bad("leaves the item inconsistent")
	}
	switch e.Reason {
	case ReasonUser, ReasonAutoPause, ReasonReopen:
		if !e.RecordedAt.Equal(e.OccurredAt) {
			return bad("was recorded at a different time")
		}
	case ReasonDayEnd:
	default:
		return bad("has an unknown reason")
	}
	if e.Before == nil {
		a := e.After
		if e.Reason != ReasonUser || a.Status != Todo || !a.CreatedAt.Equal(e.OccurredAt) || a.StartedAt != nil || a.ClosedAt != nil || a.Archived || a.UpdatedAt != nil {
			return bad("does not create a fresh item")
		}
		return nil
	}
	expected, err := e.replay()
	if err != nil || !Same(expected, e.After) {
		return bad("does not follow from the item before it")
	}
	return nil
}

// Correction tells whether the event marks a past item done after its day
// ended, which is the one change allowed once a day is closed.
func (e Event) Correction() bool {
	return e.Before != nil && e.Before.CorrectedAt == nil && e.After.CorrectedAt != nil
}

// ValidateFor checks the event against its day as well: status changes
// happen while the day is open, a day end happens exactly at midnight, and no
// edit happens after the day closed.
func (e Event) ValidateFor(day Day) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.After.DayID != day.ID {
		return fmt.Errorf("%w: event for %s belongs to another day", ErrInvalidEvidence, e.ItemID)
	}
	_, cutoff, err := day.Bounds()
	if err != nil {
		return err
	}
	switch {
	case e.Correction():
		if e.OccurredAt.Before(cutoff) {
			return fmt.Errorf("%w: %s was marked done later before its day ended", ErrInvalidEvidence, e.ItemID)
		}
		return nil
	case e.Reason == ReasonDayEnd && !e.OccurredAt.Equal(cutoff):
		return fmt.Errorf("%w: day end for %s is not at midnight", ErrInvalidEvidence, e.ItemID)
	case e.Reason != ReasonDayEnd && e.FromStatus != e.ToStatus:
		return day.RequireOpen(e.OccurredAt)
	case e.Reason != ReasonDayEnd && !e.OccurredAt.Before(cutoff):
		return fmt.Errorf("%w: event for %s happened after the day closed", ErrDayClosed, e.ItemID)
	}
	return nil
}

// replay applies the change the event describes to its before state.
func (e Event) replay() (Item, error) {
	b := *e.Before
	if b.ID != e.After.ID || b.DayID != e.After.DayID || b.Archived {
		return Item{}, ErrInvalidEvidence
	}
	expected := b.Clone()
	var err error
	switch {
	case e.Correction():
		if e.Reason != ReasonUser {
			return Item{}, ErrInvalidEvidence
		}
		err = expected.CorrectToDone(e.OccurredAt)
	case e.Reason == ReasonDayEnd:
		err = b.checkClock(e.OccurredAt)
		if !b.Status.Unfinished() {
			err = ErrInvalidEvidence
		}
		closed, recorded := e.OccurredAt, e.RecordedAt
		expected.Status, expected.ClosedAt, expected.UpdatedAt = Incomplete, &closed, &recorded
	case e.After.Archived:
		err = expected.Archive(e.OccurredAt)
	case e.FromStatus != e.ToStatus:
		wantReason := ReasonUser
		switch {
		case e.FromStatus == Done:
			wantReason = ReasonReopen
		case e.FromStatus == InProgress && e.ToStatus == Paused && e.Reason == ReasonAutoPause:
			wantReason = ReasonAutoPause
		}
		if e.Reason != wantReason || !reachable(b, e.ToStatus) {
			return Item{}, ErrInvalidEvidence
		}
		err = expected.move(e.ToStatus, e.OccurredAt)
	default:
		if e.Reason != ReasonUser {
			return Item{}, ErrInvalidEvidence
		}
		err = expected.Edit(e.After.Title, e.After.DurationMinutes, e.OccurredAt)
	}
	if err != nil {
		return Item{}, err
	}
	return expected, nil
}

// reachable reports whether some action takes item to the status to.
func reachable(item Item, to Status) bool {
	for _, a := range []Action{Start, Pause, Resume, Finish, Skip, Unskip, Reopen} {
		if got, err := item.target(a); err == nil && got == to {
			return true
		}
	}
	return false
}
