package domain

import (
	"errors"
	"testing"
	"time"
)

func TestCaptureRoundsToTheNearestSecondInUTC(t *testing.T) {
	zone := time.FixedZone("IST", 5*3600+1800)
	cases := []struct{ in, want time.Time }{
		{time.Date(2026, 11, 17, 9, 0, 0, 499_999_999, zone), time.Date(2026, 11, 17, 3, 30, 0, 0, time.UTC)},
		{time.Date(2026, 11, 17, 9, 0, 0, 500_000_000, zone), time.Date(2026, 11, 17, 3, 30, 1, 0, time.UTC)},
		{time.Date(2026, 11, 17, 9, 0, 59, 900_000_000, zone), time.Date(2026, 11, 17, 3, 31, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		if got := Capture(c.in); !got.Equal(c.want) || got.Location() != time.UTC {
			t.Errorf("Capture(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	// Rounding up onto a quarter hour could cross midnight, so it rounds down.
	for _, quarter := range []time.Time{
		time.Date(2026, 11, 17, 23, 59, 59, 600_000_000, time.UTC),
		time.Date(2026, 11, 17, 18, 29, 59, 900_000_000, time.UTC), // midnight in India
		time.Date(2026, 11, 17, 9, 44, 59, 500_000_000, time.UTC),
	} {
		got := Capture(quarter)
		if got.Unix()%(15*60) == 0 || got.After(quarter) {
			t.Errorf("Capture(%v) = %v rounded up onto a quarter hour", quarter, got)
		}
	}
	if !Capture(time.Time{}).IsZero() {
		t.Error("a zero time must stay zero")
	}
}

func TestSpan(t *testing.T) {
	start := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	if got, err := Span(start, start.Add(90*time.Second)); err != nil || got != int64(90*time.Second) {
		t.Fatalf("got %d, %v", got, err)
	}
	if _, err := Span(start, start.Add(-time.Second)); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal("an end before the start must be refused", err)
	}
	far := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := Span(time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), far); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal("a span too large for nanoseconds must be refused", err)
	}
}
