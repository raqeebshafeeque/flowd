package planner

import (
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func TestSpanWritesWholeSeconds(t *testing.T) {
	cases := map[int64]string{0: "0m", 38: "38s", 605: "10m5s", 3600: "1h", 5706: "1h35m6s", 90000: "25h"}
	for seconds, want := range cases {
		if got := Span(seconds * int64(time.Second)); got != want {
			t.Errorf("Span(%ds) = %q, want %q", seconds, got, want)
		}
	}
}

func TestEffortAndPaused(t *testing.T) {
	item := ItemView{Item: domain.Item{DurationMinutes: 90}}
	if Effort(item) != "1h30m" || Paused(item) != "-" {
		t.Fatalf("not started: %q %q", Effort(item), Paused(item))
	}
	started := time.Now()
	paused := int64(20 * time.Minute)
	item.StartedAt, item.WorkNS, item.PausedNS = &started, int64(10*time.Minute), &paused
	if Effort(item) != "10m / 1h30m" || Paused(item) != "20m" {
		t.Fatalf("started: %q %q", Effort(item), Paused(item))
	}
}

func TestLabels(t *testing.T) {
	if Label(domain.InProgress) != "IN PROGRESS" || Words(domain.InProgress) != "in progress" || Label(domain.Done) != "DONE" {
		t.Fatal("unexpected labels")
	}
}
