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
func Capture(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC().Round(time.Second)
}

// Span returns the exact time from start to end in nanoseconds. Unlike
// time.Time.Sub it refuses instead of quietly capping a huge result.
func Span(start, end time.Time) (int64, error) {
	if !ValidInstant(start) || !ValidInstant(end) || end.Before(start) {
		return 0, fmt.Errorf("%w: times are out of order", ErrInvalidEvidence)
	}
	span := end.Sub(start)
	if !start.Add(span).Equal(end) {
		return 0, fmt.Errorf("%w: time span is too large", ErrInvalidEvidence)
	}
	return int64(span), nil
}
