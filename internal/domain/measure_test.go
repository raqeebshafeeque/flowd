package domain

import (
	"errors"
	"testing"
	"time"
)

func minutes(n int64) int64 { return n * int64(time.Minute) }

func TestWorkAndPaused(t *testing.T) {
	// Ten minutes running, twenty paused, five running: 15 minutes of work and
	// 20 paused, as the docs describe.
	it, events := history(t, Start, "09:00:00", Pause, "09:10:00", Resume, "09:30:00", Finish, "09:35:00")
	m, err := Measure(testDay, it, events, at("12:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	if m.WorkNS != minutes(15) || m.PausedNS == nil || *m.PausedNS != minutes(20) {
		t.Fatalf("got work %v paused %v", time.Duration(m.WorkNS), m.PausedNS)
	}
}

func TestRunningItemGrowsUntilNow(t *testing.T) {
	it, events := history(t, Start, "09:00:00", Pause, "09:10:00", Resume, "09:30:00")
	m, err := Measure(testDay, it, events, at("09:45:00").Add(400*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if m.WorkNS != minutes(25) || *m.PausedNS != minutes(20) {
		t.Fatalf("got work %v paused %v", time.Duration(m.WorkNS), time.Duration(*m.PausedNS))
	}
}

func TestAPausedItemKeepsCountingUntilNow(t *testing.T) {
	it, events := history(t, Start, "09:00:00", Pause, "09:10:00")
	m, err := Measure(testDay, it, events, at("09:40:00"))
	if err != nil || m.WorkNS != minutes(10) || *m.PausedNS != minutes(30) {
		t.Fatalf("got %+v, %v", m, err)
	}
}

func TestAStartedItemThatNeverPausedHasNoPausedTime(t *testing.T) {
	it, events := history(t, Start, "09:00:00", Finish, "09:30:00")
	m, err := Measure(testDay, it, events, at("12:00:00"))
	if err != nil || m.PausedNS == nil || *m.PausedNS != 0 {
		t.Fatalf("got %+v, %v", m, err)
	}
}

func TestNothingCountsPastMidnight(t *testing.T) {
	it, events := history(t, Start, "23:00:00")
	m, err := Measure(testDay, it, events, at("23:59:59").Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if m.WorkNS != minutes(60) || *m.PausedNS != 0 {
		t.Fatalf("got work %v paused %v, want work capped at midnight", time.Duration(m.WorkNS), time.Duration(*m.PausedNS))
	}
	before := it.Clone()
	cutoff, noticed := at("23:59:59").Add(time.Second), at("23:59:59").Add(2*time.Hour)
	_, _ = it.CloseDay(testDay, noticed)
	events = append(events, Record(&before, it, cutoff, noticed, ReasonDayEnd))
	events[len(events)-1].Seq = int64(len(events))
	if m, err := Measure(testDay, it, events, noticed); err != nil || m.WorkNS != minutes(60) {
		t.Fatalf("after closing: %+v, %v", m, err)
	}
}

func TestNeverStartedHasNoPausedTime(t *testing.T) {
	it, events := history(t)
	m, err := Measure(testDay, it, events, at("12:00:00"))
	if err != nil || m.WorkNS != 0 || m.PausedNS != nil {
		t.Fatalf("got %+v, %v", m, err)
	}
	skipped, events := history(t, Skip, "09:00:00")
	if m, err := Measure(testDay, skipped, events, at("12:00:00")); err != nil || m.WorkNS != 0 || m.PausedNS != nil {
		t.Fatalf("a skipped item that never started: %+v, %v", m, err)
	}
}

// Time spent done before a reopen, or skipped before an unskip, was not a
// pause, so it is not counted as one.
func TestOnlyRealPausesCountAsPaused(t *testing.T) {
	it, events := history(t, Start, "09:00:00", Finish, "09:20:00", Reopen, "10:00:00", Finish, "10:10:00")
	m, err := Measure(testDay, it, events, at("12:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	if m.WorkNS != minutes(30) || *m.PausedNS != 0 {
		t.Fatalf("got work %v paused %v", time.Duration(m.WorkNS), time.Duration(*m.PausedNS))
	}
	it, events = history(t, Start, "09:00:00", Pause, "09:10:00", Skip, "09:15:00", Unskip, "10:00:00", Resume, "10:05:00", Finish, "10:15:00")
	m, err = Measure(testDay, it, events, at("12:00:00"))
	if err != nil || m.WorkNS != minutes(20) || *m.PausedNS != minutes(10) {
		t.Fatalf("got %+v, %v", m, err)
	}
}

func TestArchivingStopsWork(t *testing.T) {
	it, events := history(t, Start, "09:00:00")
	before := it.Clone()
	_ = it.Archive(at("09:20:00"))
	events = append(events, Record(&before, it, at("09:20:00"), at("09:20:00"), ReasonUser))
	events[len(events)-1].Seq = int64(len(events))
	m, err := Measure(testDay, it, events, at("12:00:00"))
	if err != nil || m.WorkNS != minutes(20) || *m.PausedNS != 0 {
		t.Fatalf("got %+v, %v", m, err)
	}
}

func TestBrokenHistoryIsRefused(t *testing.T) {
	it, events := history(t, Start, "09:00:00", Pause, "09:10:00")
	cases := map[string]func([]Event) []Event{
		"missing the latest event": func(e []Event) []Event { return e[:len(e)-1] },
		"missing the creation":     func(e []Event) []Event { return e[1:] },
		"out of order":             func(e []Event) []Event { return []Event{e[0], e[2], e[1]} },
		"repeated sequence":        func(e []Event) []Event { e[2].Seq = e[1].Seq; return e },
		"another item's event": func(e []Event) []Event {
			e[1].ItemID, e[1].After.ID, e[1].Before.ID = "it_other", "it_other", "it_other"
			return e
		},
	}
	for name, breakIt := range cases {
		copied := append([]Event(nil), events...)
		for i := range copied {
			copied[i] = Record(copied[i].Before, copied[i].After, copied[i].OccurredAt, copied[i].RecordedAt, copied[i].Reason)
			copied[i].Seq = events[i].Seq
		}
		if _, err := Measure(testDay, it, breakIt(copied), at("12:00:00")); !errors.Is(err, ErrInvalidEvidence) {
			t.Errorf("%s was accepted: %v", name, err)
		}
	}
	if _, err := Measure(testDay, it, events, at("07:00:00")); !errors.Is(err, ErrInvalidEvidence) {
		t.Error("measuring before the item existed must be refused")
	}
}

func TestSummary(t *testing.T) {
	done, doneEvents := history(t, Start, "09:00:00", Finish, "09:30:00")
	todo, _ := history(t)
	deleted := newTestItem(t, "Gone")
	_ = deleted.Archive(at("09:00:00"))
	m, err := Measure(testDay, done, doneEvents, at("12:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewSummary()
	for _, add := range []struct {
		it Item
		m  Metrics
	}{{done, m}, {todo, Metrics{}}, {deleted, Metrics{}}} {
		if err := s.Add(add.it, add.m); err != nil {
			t.Fatal(err)
		}
	}
	if s.Counts[Done] != 1 || s.Counts[Todo] != 1 || s.Counts[Skipped] != 0 || s.DurationMinutes.Int64() != 120 || s.WorkNS != minutes(30) {
		t.Fatalf("unexpected summary: %+v %v", s.Counts, s.DurationMinutes)
	}
}

// Study in the example day of docs/02-domain.md: started at 09:00, paused
// when Code started at 09:40, resumed at 11:15 and finished at 11:30.
func TestTheExampleDayInTheDocs(t *testing.T) {
	it, events := history(t, Start, "09:00:00", Pause, "09:40:00", Resume, "11:15:00", Finish, "11:30:00")
	m, err := Measure(testDay, it, events, at("12:00:00"))
	if err != nil || m.WorkNS != minutes(55) || *m.PausedNS != minutes(95) {
		t.Fatalf("got %+v, %v", m, err)
	}
}
