package domain

import (
	"fmt"
	"time"
)

// ValidInstant reports whether t is set and inside the years 0 to 9999, which
// is the range RFC 3339 text can store.
func ValidInstant(t time.Time) bool {
	return !t.IsZero() && t.UTC().Year() >= 0 && t.UTC().Year() <= 9999
}

// Capture turns a clock reading into the instant flowd records: UTC, rounded
// to the nearest whole second.
//
// It never rounds up onto a quarter hour. Every midnight in every timezone
// falls on a quarter hour, so rounding 23:59:59.6 up to 00:00:00 would move
// an action into the next day. Those readings round down instead.
func Capture(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	t = t.UTC()
	whole := t.Round(time.Second)
	if whole.After(t) && whole.Unix()%(15*60) == 0 {
		return whole.Add(-time.Second)
	}
	return whole
}

// Span returns the time from start to end in nanoseconds, refusing times
// out of order.
func Span(start, end time.Time) (int64, error) {
	if !ValidInstant(start) || !ValidInstant(end) || end.Before(start) {
		return 0, fmt.Errorf("%w: times are out of order", ErrInvalidEvidence)
	}
	return int64(end.Sub(start)), nil
}
