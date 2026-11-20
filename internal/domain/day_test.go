package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewDay(t *testing.T) {
	day, err := NewDay("2026-11-17", "Asia/Kolkata", at("08:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	if day.Date != "2026-11-17" || day.Timezone != "Asia/Kolkata" || day.ID == "" {
		t.Fatalf("unexpected day: %+v", day)
	}
	for _, bad := range []Day{
		{Date: "2026-02-30", Timezone: "UTC"},
		{Date: "17-11-2026", Timezone: "UTC"},
		{Date: "2026-11-17", Timezone: "Local"},
		{Date: "2026-11-17", Timezone: ""},
		{Date: "2026-11-17", Timezone: "Mars/Olympus"},
	} {
		if _, err := NewDay(bad.Date, bad.Timezone, at("08:00:00")); !errors.Is(err, ErrInvalidDate) && !errors.Is(err, ErrInvalidZone) {
			t.Errorf("%+v was accepted: %v", bad, err)
		}
	}
}

func TestBoundsFollowTheDaysOwnMidnight(t *testing.T) {
	cases := []struct {
		date, zone string
		start      string
		hours      float64
	}{
		{"2026-11-17", "UTC", "2026-11-17T00:00:00Z", 24},
		{"2026-11-17", "Asia/Kolkata", "2026-11-16T18:30:00Z", 24},
		{"2026-03-08", "America/New_York", "2026-03-08T05:00:00Z", 23},
		{"2026-11-01", "America/New_York", "2026-11-01T04:00:00Z", 25},
		{"2026-03-29", "Europe/London", "2026-03-29T00:00:00Z", 23},
	}
	for _, c := range cases {
		start, end, err := Day{Date: c.date, Timezone: c.zone}.Bounds()
		if err != nil {
			t.Fatalf("%s %s: %v", c.date, c.zone, err)
		}
		if start.Format(time.RFC3339) != c.start || end.Sub(start).Hours() != c.hours {
			t.Errorf("%s %s: got %s lasting %v hours, want %s lasting %v", c.date, c.zone, start.Format(time.RFC3339), end.Sub(start).Hours(), c.start, c.hours)
		}
	}
}

func TestDaysWithoutAMidnightAreRefused(t *testing.T) {
	// Santiago skipped midnight when it moved its clocks on 2026-09-06.
	if _, _, err := (Day{Date: "2026-09-06", Timezone: "America/Santiago"}).Bounds(); !errors.Is(err, ErrInvalidDate) {
		t.Fatal("a skipped midnight must be refused", err)
	}
}

func TestRequireOpen(t *testing.T) {
	day := Day{Date: "2026-11-17", Timezone: "UTC"}
	if err := day.RequireOpen(at("00:00:00")); err != nil {
		t.Error("the first second of the day is open", err)
	}
	if err := day.RequireOpen(at("23:59:59")); err != nil {
		t.Error("the last second of the day is open", err)
	}
	for _, closed := range []time.Time{at("00:00:00").Add(-time.Second), at("23:59:59").Add(time.Second), {}} {
		if err := day.RequireOpen(closed); !errors.Is(err, ErrDayClosed) {
			t.Errorf("%v must be closed: %v", closed, err)
		}
	}
}

func TestBoundsAreRememberedWithoutChangingTheAnswer(t *testing.T) {
	for _, day := range []Day{{Date: "2026-11-17", Timezone: "UTC"}, {Date: "2026-03-08", Timezone: "America/New_York"}, {Date: "2026-02-30", Timezone: "UTC"}, {Date: "2026-11-17", Timezone: "Mars/Olympus"}} {
		wantStart, wantEnd, wantErr := day.findBounds()
		for i := 0; i < 3; i++ {
			start, end, err := day.Bounds()
			if !start.Equal(wantStart) || !end.Equal(wantEnd) || (err == nil) != (wantErr == nil) {
				t.Fatalf("%+v: remembered bounds differ", day)
			}
		}
	}
}
