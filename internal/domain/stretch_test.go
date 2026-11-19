package domain

import (
	"testing"
	"time"
)

// span writes a stretch as from-to and r or p, like 09:00-09:10r.
func span(s Stretch) string {
	kind := "p"
	if s.Running {
		kind = "r"
	}
	return s.From.Format("15:04") + "-" + s.To.Format("15:04") + kind
}

func spans(stretches []Stretch) []string {
	out := []string{}
	for _, s := range stretches {
		out = append(out, span(s))
	}
	return out
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStretchesFollowTheHistory(t *testing.T) {
	_, events := history(t, Start, "09:00:00", Pause, "09:10:00", Resume, "09:30:00", Finish, "09:35:00")
	stretches, breaks := Stretches(testDay, events, at("12:00:00"))
	if want := []string{"09:00-09:10r", "09:10-09:30p", "09:30-09:35r"}; !same(spans(stretches), want) {
		t.Fatalf("got %v, want %v", spans(stretches), want)
	}
	if breaks.Stops != 1 || breaks.Switches != 0 || breaks.Total() != 1 {
		t.Fatalf("a pause someone asked for is a stop: %+v", breaks)
	}
}

// A pause because another item started is a switch, not a stop.
func TestAPauseFromStartingSomethingElseIsASwitch(t *testing.T) {
	_, events := history(t, Start, "09:00:00", Pause, "09:40:00")
	events[len(events)-1].Reason = ReasonAutoPause
	_, breaks := Stretches(testDay, events, at("12:00:00"))
	if breaks.Switches != 1 || breaks.Stops != 0 {
		t.Fatalf("got %+v", breaks)
	}
}

// Work still going on runs until now, and never past midnight.
func TestAnOpenStretchRunsUntilNow(t *testing.T) {
	_, events := history(t, Start, "23:00:00")
	if got := spans(firstOf(Stretches(testDay, events, at("23:30:00")))); !same(got, []string{"23:00-23:30r"}) {
		t.Fatalf("got %v", got)
	}
	stretches, _ := Stretches(testDay, events, at("23:59:59").Add(3*time.Hour))
	if end := stretches[0].To; !end.Equal(at("23:59:59").Add(time.Second)) {
		t.Fatalf("a stretch must stop at midnight, got %v", end)
	}
}

// Finishing, skipping and the time an item is done or skipped are neither
// work nor a pause, and finishing is not an interruption.
func TestOnlyRunningAndPausedCount(t *testing.T) {
	_, events := history(t, Start, "09:00:00", Finish, "09:20:00", Reopen, "10:00:00", Finish, "10:10:00")
	stretches, breaks := Stretches(testDay, events, at("12:00:00"))
	if want := []string{"09:00-09:20r", "10:00-10:10r"}; !same(spans(stretches), want) || breaks.Total() != 0 {
		t.Fatalf("got %v %+v", spans(stretches), breaks)
	}
	_, events = history(t, Start, "09:00:00", Pause, "09:10:00", Skip, "09:15:00", Unskip, "10:00:00", Resume, "10:05:00", Finish, "10:15:00")
	stretches, _ = Stretches(testDay, events, at("12:00:00"))
	if want := []string{"09:00-09:10r", "09:10-09:15p", "10:00-10:05p", "10:05-10:15r"}; !same(spans(stretches), want) {
		t.Fatalf("got %v, want %v", spans(stretches), want)
	}
}

func TestAnItemNeverStartedHasNoStretches(t *testing.T) {
	_, events := history(t)
	if stretches, breaks := Stretches(testDay, events, at("12:00:00")); len(stretches) != 0 || breaks.Total() != 0 {
		t.Fatalf("got %v %+v", stretches, breaks)
	}
}

func firstOf(stretches []Stretch, _ Breaks) []Stretch { return stretches }
