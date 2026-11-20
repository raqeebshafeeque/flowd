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
	Incomplete Status = "incomplete"
)

// Statuses are every status, in the order a day goes through them.
var Statuses = []Status{Todo, InProgress, Paused, Done, Incomplete}

// Valid reports whether s is one of the five statuses.
func (s Status) Valid() bool { return slices.Contains(Statuses, s) }

// Unfinished reports whether work on the item can still happen today.
func (s Status) Unfinished() bool { return s == Todo || s == InProgress || s == Paused }

// Action is something a person does to an item. Start works on a todo,
// paused or done item, so it also resumes and reopens.
type Action string

const (
	Start  Action = "start"
	Pause  Action = "pause"
	Finish Action = "finish"
)

var (
	ErrInvalidTitle      = errors.New("invalid title")
	ErrIllegalTransition = errors.New("illegal status change")
	ErrInvalidEvidence   = errors.New("invalid recorded evidence")
	ErrAlreadyRunning    = errors.New("another item is already running")
)

// MaxTitleLength is the longest title in characters.
const MaxTitleLength = 120

// Item is the saved state of one effort. Work and paused time are not part of
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
	// CarriedFrom is the item on an earlier day this one was copied from,
	// or nil. It is set when the item is created and never changes.
	CarriedFrom *ID `json:"carried_from"`
	// CorrectedAt is when an item left incomplete was marked done after its
	// day had ended, or nil. Its work is still only what was tracked.
	CorrectedAt *time.Time `json:"corrected_at"`
}

// RecentDays is how far back an incomplete item can be carried or marked
// done: the seven days before.
const RecentDays = 7

// Leftover tells whether an item was left incomplete and is not deleted, so
// it can be carried to a later day or marked done after all.
func (it Item) Leftover() bool { return !it.Archived && it.Status == Incomplete }

// CorrectToDone marks an item that was left incomplete as done, at the
// captured instant now, after its day has ended. Only the status changes: it
// finishes when it was closed, no work or start is made up, and the
// correction is recorded so it is never mistaken for a finish made at the
// time. It is done once.
func (it *Item) CorrectToDone(now time.Time) error {
	if !it.Leftover() || it.CorrectedAt != nil {
		return fmt.Errorf("%w: only an incomplete item can be marked done later", ErrIllegalTransition)
	}
	if err := it.checkClock(now); err != nil {
		return err
	}
	stamp := now.UTC()
	finished := it.ClosedAt.UTC()
	it.Status, it.FinishedAt, it.CorrectedAt, it.UpdatedAt = Done, &finished, &stamp, &stamp
	return nil
}

// CarryTo copies an incomplete item onto another day as a fresh todo item
// with the same title and duration, remembering where it came from. The
// original is not touched.
func (it Item) CarryTo(dayID ID, now time.Time) (Item, error) {
	if !it.Leftover() {
		return Item{}, fmt.Errorf("%w: only an incomplete item can be carried", ErrIllegalTransition)
	}
	copied, err := NewItem(dayID, it.Title, it.DurationMinutes, now)
	if err != nil {
		return Item{}, err
	}
	from := it.ID
	copied.CarriedFrom = &from
	return copied, nil
}

// ValidateTitle checks a new or changed title: one line of 1 to 120
// visible characters, with no hidden or direction-changing ones.
func ValidateTitle(title string) error {
	if err := checkTitle(title); err != nil {
		return err
	}
	if utf8.RuneCountInString(strings.TrimSpace(title)) > MaxTitleLength {
		return fmt.Errorf("%w: use one line of 1 to %d characters", ErrInvalidTitle, MaxTitleLength)
	}
	if problem := CheckText(title); problem != nil {
		return fmt.Errorf("%w: %w", ErrInvalidTitle, problem)
	}
	return nil
}

// checkTitle is the shape every saved title has, even one saved by an older
// version and shown with placeholders today: valid text on one line.
func checkTitle(title string) error {
	if !utf8.ValidString(title) || strings.TrimSpace(title) == "" || strings.ContainsAny(title, "\r\n") {
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
	if it.CarriedFrom != nil {
		from := *it.CarriedFrom
		it.CarriedFrom = &from
	}
	it.CorrectedAt = copyTime(it.CorrectedAt)
	it.StartedAt = copyTime(it.StartedAt)
	it.FinishedAt = copyTime(it.FinishedAt)
	it.ClosedAt = copyTime(it.ClosedAt)
	it.UpdatedAt = copyTime(it.UpdatedAt)
	return it
}

// Can tells whether the action fits the item as it is now. It is what tab
// completion, the picker and the workspace footer offer, so they never
// suggest something that would be refused.
func (it Item) Can(a Action) bool {
	_, err := it.target(a)
	return err == nil && !it.Archived
}

// target is the status an action leads to from the item's current state.
func (it Item) target(a Action) (Status, error) {
	switch {
	case a == Start && (it.Status == Todo || it.Status == Paused || it.Status == Done):
		return InProgress, nil
	case a == Pause && it.Status == InProgress:
		return Paused, nil
	case a == Finish && (it.Status == InProgress || it.Status == Paused):
		return Done, nil
	}
	return "", fmt.Errorf("%w: cannot %s an item that is %s", ErrIllegalTransition, a, strings.ReplaceAll(string(it.Status), "_", " "))
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
	case Paused:
		it.ClosedAt = nil
	case Done:
		it.FinishedAt = &stamp
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
	if err := checkTitle(it.Title); err != nil {
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
	case it.CorrectedAt != nil && (it.Status != Done || !ValidInstant(*it.CorrectedAt) || it.CorrectedAt.Before(it.CreatedAt)):
		return bad("has a correction that does not add up")
	case (it.Status == InProgress || it.Status == Paused || (it.Status == Done && it.CorrectedAt == nil)) && it.StartedAt == nil:
		return bad("has worked without a start")
	case it.Status == Todo && it.StartedAt != nil:
		return bad("is todo but has started")
	case it.Status == Done && (it.FinishedAt == nil || it.ClosedAt == nil || !it.FinishedAt.Equal(*it.ClosedAt)):
		return bad("is done without a finish")
	case it.Status != Done && it.FinishedAt != nil:
		return bad("has a finish but is not done")
	case (it.Status == Incomplete || it.Archived) && it.ClosedAt == nil:
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
