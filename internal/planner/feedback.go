package planner

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// ErrInput means a request was malformed, as opposed to refused.
var ErrInput = errors.New("invalid input")

// Feedback is how every frontend explains an error: a stable code for
// scripts, an exit status, one plain sentence and an optional hint.
type Feedback struct {
	Code    string
	Exit    int
	Summary string
	Hint    string
}

// explained attaches a summary and hint to an error without hiding its cause.
type explained struct {
	cause   error
	summary string
	hint    string
}

func (e *explained) Error() string { return e.summary }
func (e *explained) Unwrap() error { return e.cause }

// InputError reports malformed input. cause may be nil.
func InputError(cause error, summary, hint string) error {
	return &explained{cause: errors.Join(ErrInput, cause), summary: summary, hint: hint}
}

// Refusal reports a well formed request that the rules do not allow.
func Refusal(cause error, summary, hint string) error {
	return &explained{cause: cause, summary: summary, hint: hint}
}

// kinds maps causes to codes, from the most specific to the most general.
var kinds = []struct {
	cause   error
	code    string
	exit    int
	summary string
	hint    string
}{
	{context.Canceled, "ERR_CANCELLED", 130, "Stopped", ""},
	{store.ErrIncompleteRepair, "ERR_REPAIR", 1, "A repair was interrupted", "Run flowd doctor --repair to finish it"},
	{store.ErrCorrupt, "ERR_CORRUPT", 1, "The database is damaged", "Run flowd doctor, and restore the backup with flowd doctor --repair"},
	{store.ErrVersion, "ERR_VERSION", 1, "The database was made by another version of flowd", "Use a matching flowd, and do not delete the file"},
	{store.ErrLocked, "ERR_LOCKED", 1, "Another flowd is using the database", "Wait for it to finish and try again"},
	{store.ErrDiskFull, "ERR_DISK_FULL", 1, "The disk is full", "Free some space. Everything saved before is still there"},
	{store.ErrPermissions, "ERR_PERMISSION", 1, "The database is not private to you", "Make it readable only by you with chmod 600"},
	{store.ErrInvalidPath, "ERR_PATH", 2, "The database path is not a plain file", "Point --db at a regular file"},
	{ErrPastDay, "ERR_PAST_DAY", 2, "Past days can only be looked at", "Plan new work for today or a later date"},
	{domain.ErrUnsafeText, "ERR_TITLE", 2, "Titles cannot hold hidden or direction-changing characters", ""},
	{domain.ErrInvalidTitle, "ERR_TITLE", 2, "Titles are one line of 1 to 120 characters", ""},
	{ErrDurationExceedsDay, "ERR_DURATION_LIMIT", 2, "The duration is longer than the time left on this day", ""},
	{domain.ErrInvalidDuration, "ERR_DURATION", 2, "Durations are whole minutes, like 30m, 2h or 1h30m", ""},
	{ErrInput, "ERR_USAGE", 2, "That command is not valid", "See flowd --help"},
	{ErrStale, "ERR_STALE", 1, "The item changed since it was read", "Look at it again before changing it"},
	{store.ErrStale, "ERR_STALE", 1, "The item changed since it was read", "Look at it again before changing it"},
	{store.ErrNotFound, "ERR_NOT_FOUND", 1, "No such item on this day", "Check the item number or ID with flowd list"},
	{domain.ErrDayClosed, "ERR_DAY_CLOSED", 1, "That day is not open", "Only today's items can be started, paused, finished or skipped"},
	{domain.ErrAlreadyRunning, "ERR_ILLEGAL_TRANSITION", 1, "Another item is already running", ""},
	{domain.ErrIllegalTransition, "ERR_ILLEGAL_TRANSITION", 1, "That item cannot do that now", ""},
	{ErrStillUnknown, "ERR_UNCONFIRMED", 1, "It is unknown whether the change was saved", "Check the item before trying again"},
	{domain.ErrInvalidEvidence, "ERR_STATE", 1, "The saved history does not add up", "Run flowd doctor and check the system clock"},
}

// Describe turns any error into Feedback. Explanations attached with
// InputError or Refusal win over the generic wording.
func Describe(err error) Feedback {
	f := Feedback{Code: "ERR_INTERNAL", Exit: 1, Summary: "Something went wrong", Hint: err.Error()}
	for _, k := range kinds {
		if errors.Is(err, k.cause) {
			f = Feedback{Code: k.code, Exit: k.exit, Summary: k.summary, Hint: k.hint}
			if f.Hint == "" {
				f.Hint = detail(err, k.cause)
			}
			var problem *domain.TextProblem
			if errors.As(err, &problem) {
				f.Hint = capitalize(problem.Error())
			}
			break
		}
	}
	var e *explained
	if errors.As(err, &e) {
		f.Summary = e.summary
		if e.hint != "" {
			f.Hint = e.hint
		}
	}
	return f
}

func quote(text string) string { return strconv.Quote(text) }

// detail is the part of an error message after its cause, as a sentence,
// for example "Cannot start an item that is in progress".
func detail(err, cause error) string {
	text := err.Error()
	_, rest, found := strings.Cut(text, cause.Error()+": ")
	if !found {
		return ""
	}
	return capitalize(rest)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
