package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Status is where an item is in its day.
type Status string

const (
	Todo       Status = "todo"
	InProgress Status = "in_progress"
	Paused     Status = "paused"
	Done       Status = "done"
	Skipped    Status = "skipped"
	Incomplete Status = "incomplete"
)

// Valid reports whether s is one of the six statuses.
func (s Status) Valid() bool {
	switch s {
	case Todo, InProgress, Paused, Done, Skipped, Incomplete:
		return true
	}
	return false
}

// Unfinished reports whether work on the item can still happen today.
func (s Status) Unfinished() bool { return s == Todo || s == InProgress || s == Paused }

// Action is something a person does to an item.
type Action string

const (
	Start  Action = "start"
	Pause  Action = "pause"
	Resume Action = "resume"
	Finish Action = "finish"
	Skip   Action = "skip"
	Unskip Action = "unskip"
	Reopen Action = "reopen"
)

var (
	ErrInvalidTitle      = errors.New("invalid title")
	ErrIllegalTransition = errors.New("illegal status change")
	ErrInvalidEvidence   = errors.New("invalid recorded evidence")
	ErrAlreadyRunning    = errors.New("another item is already running")
)

// MaxTitleLength is the longest title in characters.
const MaxTitleLength = 120

// Item is the saved state of one effort. Work and elapsed time are not part of
// it, they are measured from its events.
type Item struct {
	ID              ID         `json:"id"`
	DayID           ID         `json:"day_id"`
	Title           string     `json:"title"`
	Status          Status     `json:"status"`
	DurationMinutes int64      `json:"duration_minutes"`
	StartedAt       *time.Time `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	ClosedAt        *time.Time `json:"closed_at"`
	Archived        bool       `json:"archived"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
}

// ValidateTitle checks that a title is one line of 1 to 120 characters.
func ValidateTitle(title string) error {
	trimmed := strings.TrimSpace(title)
	if !utf8.ValidString(title) || trimmed == "" || utf8.RuneCountInString(trimmed) > MaxTitleLength || strings.ContainsAny(title, "\r\n") {
		return fmt.Errorf("%w: use one line of 1 to %d characters", ErrInvalidTitle, MaxTitleLength)
	}
	return nil
}

// NewItem plans a todo item on a day at the captured instant now.
func NewItem(dayID ID, title string, minutes int64, now time.Time) (Item, error) {
	if err := ValidateTitle(title); err != nil {
		return Item{}, err
	}
	if minutes <= 0 {
		return Item{}, ErrInvalidDuration
	}
	if !ValidInstant(now) {
		return Item{}, fmt.Errorf("%w: invalid creation time", ErrInvalidEvidence)
	}
	return Item{ID: NewID("it"), DayID: dayID, Title: title, Status: Todo, DurationMinutes: minutes, CreatedAt: Capture(now)}, nil
}

// Clone returns a copy that shares no time pointers with it.
func (it Item) Clone() Item {
	copyTime := func(t *time.Time) *time.Time {
		if t == nil {
			return nil
		}
		v := t.UTC()
		return &v
	}
	it.CreatedAt = it.CreatedAt.UTC()
	it.StartedAt = copyTime(it.StartedAt)
	it.FinishedAt = copyTime(it.FinishedAt)
	it.ClosedAt = copyTime(it.ClosedAt)
	it.UpdatedAt = copyTime(it.UpdatedAt)
	return it
}

// target is the status an action leads to from the item's current state.
func (it Item) target(a Action) (Status, error) {
	refuse := func() (Status, error) {
		return "", fmt.Errorf("%w: cannot %s a %s item", ErrIllegalTransition, a, it.Status)
	}
	switch {
	case a == Start && it.Status == Todo:
		return InProgress, nil
	case a == Pause && it.Status == InProgress:
		return Paused, nil
	case a == Resume && it.Status == Paused:
		return InProgress, nil
	case a == Finish && (it.Status == InProgress || it.Status == Paused):
		return Done, nil
	case a == Skip && it.Status.Unfinished():
		return Skipped, nil
	case a == Unskip && it.Status == Skipped:
		if it.StartedAt == nil {
			return Todo, nil
		}
		return Paused, nil
	case a == Reopen && it.Status == Done:
		return InProgress, nil
	}
	return refuse()
}

// Apply performs an action at the captured instant now. The item's day has
// to be open at that instant. A refusal leaves the item exactly as it was.
func (it *Item) Apply(a Action, now time.Time, day Day) error {
	if it.Archived || it.Status == Incomplete {
		return fmt.Errorf("%w: the item is closed", ErrIllegalTransition)
	}
	if err := day.RequireOpen(now); err != nil {
		return err
	}
	to, err := it.target(a)
	if err != nil {
		return err
	}
	return it.move(to, now)
}

// move changes the status and its timestamps. Finish means today's work on it
// ended, nothing more. The first start is never changed once set.
func (it *Item) move(to Status, now time.Time) error {
	if err := it.checkClock(now); err != nil {
		return err
	}
	stamp := now.UTC()
	switch to {
	case InProgress:
		if it.StartedAt == nil {
			it.StartedAt = &stamp
		}
		it.FinishedAt = nil
		it.ClosedAt = nil
	case Todo, Paused:
		it.ClosedAt = nil
	case Done:
		it.FinishedAt = &stamp
		it.ClosedAt = &stamp
	case Skipped:
		it.ClosedAt = &stamp
	default:
		return fmt.Errorf("%w: %s is not reached by an action", ErrIllegalTransition, to)
	}
	it.Status = to
	it.UpdatedAt = &stamp
	return nil
}

// CloseDay makes an unfinished item incomplete once its day has ended. The
// item closes at exactly midnight even when this runs later, at recorded. It
// reports whether anything changed. No finish or start is ever invented.
func (it *Item) CloseDay(day Day, recorded time.Time) (bool, error) {
	_, cutoff, err := day.Bounds()
	if err != nil {
		return false, err
	}
	if recorded.Before(cutoff) || it.Archived || !it.Status.Unfinished() {
		return false, nil
	}
	if err := it.checkClock(cutoff); err != nil {
		return false, err
	}
	closed, noticed := cutoff.UTC(), recorded.UTC()
	it.Status, it.ClosedAt, it.UpdatedAt = Incomplete, &closed, &noticed
	return true, nil
}

// Edit changes the title and Duration. Editing never changes the status, the
// recorded times or the item's place in the order.
func (it *Item) Edit(title string, minutes int64, now time.Time) error {
	if it.Archived {
		return fmt.Errorf("%w: the item is deleted", ErrIllegalTransition)
	}
	if err := ValidateTitle(title); err != nil {
		return err
	}
	if minutes <= 0 {
		return ErrInvalidDuration
	}
	if err := it.checkClock(now); err != nil {
		return err
	}
	stamp := now.UTC()
	it.Title, it.DurationMinutes, it.UpdatedAt = title, minutes, &stamp
	return nil
}

// Archive hides the item. Its status and history stay, and a running item
// stops collecting work from this moment.
func (it *Item) Archive(now time.Time) error {
	if it.Archived {
		return nil
	}
	if err := it.checkClock(now); err != nil {
		return err
	}
	stamp := now.UTC()
	it.Archived = true
	it.UpdatedAt = &stamp
	if it.ClosedAt == nil {
		it.ClosedAt = &stamp
	}
	return nil
}

// checkClock refuses an instant earlier than anything already recorded, so a
// clock that jumped backwards cannot rewrite the past.
func (it Item) checkClock(now time.Time) error {
	for _, t := range []*time.Time{&it.CreatedAt, it.UpdatedAt, it.StartedAt, it.ClosedAt} {
		if t != nil && now.Before(*t) {
			return fmt.Errorf("%w: the clock is earlier than the saved state", ErrInvalidEvidence)
		}
	}
	if !ValidInstant(now) {
		return fmt.Errorf("%w: invalid time", ErrInvalidEvidence)
	}
	return nil
}

// Check reports whether the saved fields agree with each other.
func (it Item) Check() error {
	bad := func(why string) error { return fmt.Errorf("%w: item %s %s", ErrInvalidEvidence, it.ID, why) }
	if err := ValidateTitle(it.Title); err != nil {
		return err
	}
	if it.DurationMinutes <= 0 {
		return ErrInvalidDuration
	}
	if !it.Status.Valid() || !ValidInstant(it.CreatedAt) {
		return bad("has an unknown status or creation time")
	}
	for _, t := range []*time.Time{it.StartedAt, it.UpdatedAt, it.ClosedAt} {
		if t != nil && (!ValidInstant(*t) || t.Before(it.CreatedAt)) {
			return bad("has a time before it was created")
		}
	}
	switch {
	case it.StartedAt != nil && it.ClosedAt != nil && it.ClosedAt.Before(*it.StartedAt):
		return bad("closed before it started")
	case (it.Status == InProgress || it.Status == Paused || it.Status == Done) && it.StartedAt == nil:
		return bad("has worked without a start")
	case it.Status == Todo && it.StartedAt != nil:
		return bad("is todo but has started")
	case it.Status == Done && (it.FinishedAt == nil || it.ClosedAt == nil || !it.FinishedAt.Equal(*it.ClosedAt)):
		return bad("is done without a finish")
	case it.Status != Done && it.FinishedAt != nil:
		return bad("has a finish but is not done")
	case (it.Status == Skipped || it.Status == Incomplete || it.Archived) && it.ClosedAt == nil:
		return bad("is closed without a closing time")
	case !it.Archived && it.Status.Unfinished() && it.ClosedAt != nil:
		return bad("is open but has a closing time")
	}
	return nil
}

// CheckDay checks every item of one day and that at most one is running.
func CheckDay(items []Item) error {
	running := 0
	for _, it := range items {
		if err := it.Check(); err != nil {
			return err
		}
		if !it.Archived && it.Status == InProgress {
			running++
		}
	}
	if running > 1 {
		return ErrAlreadyRunning
	}
	return nil
}

// Compare is the one order every list uses. Started items come first, by
// their first start, then the rest by creation time, with the ID breaking any
// tie. Status, finishing and editing never change an item's place.
func Compare(a, b Item) int {
	switch {
	case (a.StartedAt == nil) != (b.StartedAt == nil):
		if a.StartedAt != nil {
			return -1
		}
		return 1
	case a.StartedAt != nil && !a.StartedAt.Equal(*b.StartedAt):
		return a.StartedAt.Compare(*b.StartedAt)
	case !a.CreatedAt.Equal(b.CreatedAt):
		return a.CreatedAt.Compare(b.CreatedAt)
	}
	return strings.Compare(string(a.ID), string(b.ID))
}

// Sort puts items in the order defined by Compare.
func Sort(items []Item) { slices.SortStableFunc(items, Compare) }
