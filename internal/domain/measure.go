package domain

import (
	"fmt"
	"time"
)

// Metrics is what really happened to an item at one moment.
//
// Work is the total time it ran. Paused is the total time it waited as
// paused after it had started. Time spent done before it was started again
// is neither. Paused is nil for an item that never started. Both are whole
// seconds, held in nanoseconds.
type Metrics struct {
	WorkNS   int64  `json:"work_ns"`
	PausedNS *int64 `json:"paused_ns"`
}

// Measure works out the metrics of item at the instant observed from its
// events, which must be its complete history in order. Nothing counts past
// the day's midnight, even if the day has not been closed yet. Broken or
// reordered history is refused, never quietly corrected.
func Measure(day Day, item Item, events []Event, observed time.Time) (Metrics, error) {
	var result Metrics
	_, cutoff, err := day.Bounds()
	if err != nil {
		return result, err
	}
	observed = Capture(observed)
	if !ValidInstant(observed) || observed.Before(item.CreatedAt) {
		return result, fmt.Errorf("%w: measured before the item existed", ErrInvalidEvidence)
	}
	if observed.After(cutoff) {
		observed = cutoff
	}
	var previous *Item
	var runningSince, pausedSince *time.Time
	var paused int64
	var lastOccurred, lastRecorded time.Time
	var lastSeq int64
	// Every stretch lies within one day, so these sums cannot overflow.
	stopRunning := func(at time.Time) error {
		span, err := Span(*runningSince, at)
		result.WorkNS += span
		runningSince = nil
		return err
	}
	stopPause := func(at time.Time) error {
		span, err := Span(*pausedSince, at)
		paused += span
		pausedSince = nil
		return err
	}
	for i, e := range events {
		if err := e.ValidateFor(day); err != nil {
			return result, err
		}
		// A correction comes after the day ended and changes nothing that is
		// measured, so it is the one event allowed past the cutoff.
		if e.ItemID != item.ID || (e.OccurredAt.After(observed) && !e.Correction()) || e.OccurredAt.Before(lastOccurred) || e.RecordedAt.Before(lastRecorded) || (i > 0 && e.Seq <= lastSeq) {
			return result, fmt.Errorf("%w: events are out of order", ErrInvalidEvidence)
		}
		if (previous == nil) != (e.Before == nil) || (previous != nil && !Same(*previous, *e.Before)) {
			return result, fmt.Errorf("%w: events do not follow each other", ErrInvalidEvidence)
		}
		wasRunning := previous != nil && previous.Status == InProgress && !previous.Archived
		isRunning := e.After.Status == InProgress && !e.After.Archived
		if wasRunning && !isRunning {
			if err := stopRunning(e.OccurredAt); err != nil {
				return result, err
			}
		}
		if !wasRunning && isRunning {
			at := e.OccurredAt
			runningSince = &at
		}
		wasPaused := previous != nil && previous.Status == Paused && !previous.Archived
		isPaused := e.After.Status == Paused && !e.After.Archived
		if wasPaused && !isPaused {
			if err := stopPause(e.OccurredAt); err != nil {
				return result, err
			}
		}
		if !wasPaused && isPaused {
			at := e.OccurredAt
			pausedSince = &at
		}
		after := e.After
		previous = &after
		lastOccurred, lastRecorded, lastSeq = e.OccurredAt, e.RecordedAt, e.Seq
	}
	if previous == nil || !Same(*previous, item) {
		return result, fmt.Errorf("%w: history does not end at the saved item", ErrInvalidEvidence)
	}
	if runningSince != nil {
		if err := stopRunning(observed); err != nil {
			return result, err
		}
	}
	if pausedSince != nil {
		if err := stopPause(observed); err != nil {
			return result, err
		}
	}
	if item.StartedAt != nil {
		span, err := Span(*item.StartedAt, observed)
		if err != nil {
			return result, err
		}
		if result.WorkNS > span-paused {
			return result, fmt.Errorf("%w: work and pauses are longer than the time since the first start", ErrInvalidEvidence)
		}
		result.PausedNS = &paused
	} else if paused != 0 {
		return result, fmt.Errorf("%w: paused before it ever started", ErrInvalidEvidence)
	}
	if result.WorkNS%int64(time.Second) != 0 || paused%int64(time.Second) != 0 {
		return result, fmt.Errorf("%w: times are not whole seconds", ErrInvalidEvidence)
	}
	return result, nil
}

// Summary adds up a day. Deleted items are left out.
type Summary struct {
	Counts          map[Status]int `json:"status_counts"`
	DurationMinutes int64          `json:"duration_minutes"`
	WorkNS          int64          `json:"work_ns"`
}

// NewSummary returns an empty summary with every status counted as zero.
func NewSummary() Summary {
	counts := map[Status]int{}
	for _, s := range Statuses {
		counts[s] = 0
	}
	return Summary{Counts: counts}
}

// Add counts one item and its metrics.
func (s *Summary) Add(item Item, m Metrics) error {
	if item.Archived {
		return nil
	}
	if !item.Status.Valid() || item.DurationMinutes <= 0 || m.WorkNS < 0 {
		return fmt.Errorf("%w: cannot add item %s to the day", ErrInvalidEvidence, item.ID)
	}
	s.Counts[item.Status]++
	s.DurationMinutes += item.DurationMinutes
	s.WorkNS += m.WorkNS
	return nil
}
