package domain

import (
	"testing"
	"time"
)

func instantAt(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestRemainingMinutes(t *testing.T) {
	utc := Day{Date: "2026-11-11", Timezone: "UTC"}
	newYork := func(date string) Day { return Day{Date: date, Timezone: "America/New_York"} }
	hour := int64(time.Hour)
	cases := []struct {
		name   string
		day    Day
		now    string
		others []Reservation
		want   int64
	}{
		{"today at 20:00 with nothing else", utc, "2026-11-11T20:00:00Z", nil, 240},
		{"today at midday", utc, "2026-11-11T12:00:00Z", nil, 720},
		{"a partial minute is not offered", utc, "2026-11-11T23:59:30Z", nil, 0},
		{"one minute left", utc, "2026-11-11T23:59:00Z", nil, 1},
		{"future date is the whole day", utc, "2026-11-10T08:00:00Z", nil, 1440},
		{"a day before it starts uses the full day even far ahead", utc, "2026-10-01T00:00:00Z", nil, 1440},
		{"a closed day has none", utc, "2026-11-12T00:00:00Z", nil, 0},
		{"exactly the midnight cutoff has none", utc, "2026-11-12T00:00:00Z", []Reservation{{Status: Todo, DurationMinutes: 5}}, 0},
		{"todo holds its full duration", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: Todo, DurationMinutes: 90}}, 150},
		{"several todos add up", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: Todo, DurationMinutes: 60}, {Status: Todo, DurationMinutes: 30}}, 150},
		{"running holds the unworked remainder", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: InProgress, DurationMinutes: 60, WorkNS: hour / 2}}, 210},
		{"paused holds the unworked remainder", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: Paused, DurationMinutes: 60, WorkNS: 45 * int64(time.Minute)}}, 225},
		{"a partial remaining minute is still held", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: InProgress, DurationMinutes: 60, WorkNS: hour - 30*int64(time.Second)}}, 239},
		{"work beyond the duration holds nothing", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: InProgress, DurationMinutes: 30, WorkNS: hour}}, 240},
		{"done holds nothing", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: Done, DurationMinutes: 600, WorkNS: hour}}, 240},
		{"skipped holds nothing", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: Skipped, DurationMinutes: 600}}, 240},
		{"incomplete holds nothing", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: Incomplete, DurationMinutes: 600}}, 240},
		{"archived holds nothing", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: Todo, DurationMinutes: 600, Archived: true}}, 240},
		{"over-planned day clamps to zero", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: Todo, DurationMinutes: 500}}, 0},
		{"exactly full is zero", utc, "2026-11-11T20:00:00Z", []Reservation{{Status: Todo, DurationMinutes: 240}}, 0},
		{"future day minus todos", utc, "2026-11-10T08:00:00Z", []Reservation{{Status: Todo, DurationMinutes: 1000}}, 440},
		{"spring forward day is 23 hours", newYork("2026-03-08"), "2026-03-07T12:00:00Z", nil, 1380},
		{"fall back day is 25 hours", newYork("2026-11-01"), "2026-10-31T12:00:00Z", nil, 1500},
		{"zone midnight, not UTC", newYork("2026-11-11"), "2026-11-11T22:00:00-05:00", nil, 120},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := RemainingMinutes(c.day, instantAt(t, c.now), c.others)
			if err != nil || got != c.want {
				t.Fatalf("got %d, %v; want %d", got, err, c.want)
			}
		})
	}
}

func TestRemainingMinutesRefusesInvalidDays(t *testing.T) {
	now := instantAt(t, "2026-11-11T12:00:00Z")
	for _, day := range []Day{{Date: "2026-02-30", Timezone: "UTC"}, {Date: "2026-11-11", Timezone: "Mars/Olympus"}, {Date: "2026-11-11", Timezone: "Local"}, {}} {
		if _, err := RemainingMinutes(day, now, nil); err == nil {
			t.Errorf("%+v accepted", day)
		}
	}
	if _, err := RemainingMinutes(Day{Date: "2026-11-11", Timezone: "UTC"}, time.Time{}, nil); err == nil {
		t.Error("zero instant accepted")
	}
}
