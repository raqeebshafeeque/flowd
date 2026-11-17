package domain

import (
	"fmt"
	"math"
	"math/big"
	"time"
)

// Metrics is what really happened to an item at one moment.
//
// Work is the total time it ran. Elapsed is the time from its first start to
// that moment, or to when it was closed, including pauses. Elapsed is nil for
// an item that never started. Both are whole seconds, held in nanoseconds.
type Metrics struct {
	WorkNS    int64  `json:"work_ns"`
	ElapsedNS *int64 `json:"elapsed_ns"`
}

// Measure works out the metrics of item at the instant observed from its
// events, which must be its complete history in order. Broken or reordered
// history is refused, never quietly corrected.
func Measure(item Item, events []Event, observed time.Time) (Metrics, error) {
	var result Metrics
	observed = Capture(observed)
	if !ValidInstant(observed) || observed.Before(item.CreatedAt) {
		return result, fmt.Errorf("%w: measured before the item existed", ErrInvalidEvidence)
	}
	var previous *Item
	var runningSince *time.Time
	var lastOccurred, lastRecorded time.Time
	var lastSeq int64
	stopRunning := func(at time.Time) error {
		span, err := Span(*runningSince, at)
		if err != nil {
			return err
		}
		if span > math.MaxInt64-result.WorkNS {
			return fmt.Errorf("%w: work is too large", ErrInvalidEvidence)
		}
		result.WorkNS += span
		runningSince = nil
		return nil
	}
	for i, e := range events {
		if err := e.Validate(); err != nil {
			return result, err
		}
		if e.ItemID != item.ID || e.OccurredAt.After(observed) || e.OccurredAt.Before(lastOccurred) || e.RecordedAt.Before(lastRecorded) || (i > 0 && e.Seq <= lastSeq) {
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
	if item.StartedAt != nil {
		end := observed
		if item.ClosedAt != nil {
			end = *item.ClosedAt
		}
		elapsed, err := Span(*item.StartedAt, end)
		if err != nil {
			return result, err
		}
		if result.WorkNS > elapsed {
			return result, fmt.Errorf("%w: work is longer than elapsed time", ErrInvalidEvidence)
		}
		result.ElapsedNS = &elapsed
	}
	if result.WorkNS%int64(time.Second) != 0 || (result.ElapsedNS != nil && *result.ElapsedNS%int64(time.Second) != 0) {
		return result, fmt.Errorf("%w: times are not whole seconds", ErrInvalidEvidence)
	}
	return result, nil
}

// Summary adds up a day. Durations are summed without any limit, so the total
// stays exact however many items there are. Deleted items are left out.
type Summary struct {
	Counts          map[Status]int `json:"status_counts"`
	DurationMinutes *big.Int       `json:"duration_minutes"`
	WorkNS          int64          `json:"work_ns"`
}

// NewSummary returns an empty summary with every status counted as zero.
func NewSummary() Summary {
	counts := map[Status]int{}
	for _, s := range []Status{Todo, InProgress, Paused, Done, Skipped, Incomplete} {
		counts[s] = 0
	}
	return Summary{Counts: counts, DurationMinutes: new(big.Int)}
}

// Add counts one item and its metrics.
func (s *Summary) Add(item Item, m Metrics) error {
	if item.Archived {
		return nil
	}
	if !item.Status.Valid() || item.DurationMinutes <= 0 || m.WorkNS < 0 || m.WorkNS > math.MaxInt64-s.WorkNS {
		return fmt.Errorf("%w: cannot add item %s to the day", ErrInvalidEvidence, item.ID)
	}
	s.Counts[item.Status]++
	s.DurationMinutes.Add(s.DurationMinutes, big.NewInt(item.DurationMinutes))
	s.WorkNS += m.WorkNS
	return nil
}
