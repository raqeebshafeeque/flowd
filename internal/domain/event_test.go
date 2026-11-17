package domain

import (
	"errors"
	"testing"
	"time"
)

// history plays actions on a new item and returns it with its events, the
// way the store will save them.
func history(t *testing.T, steps ...any) (Item, []Event) {
	t.Helper()
	it := newTestItem(t, "Study")
	events := []Event{Record(nil, it, it.CreatedAt, it.CreatedAt, ReasonUser)}
	for i := 0; i < len(steps); i += 2 {
		action, clock := steps[i].(Action), at(steps[i+1].(string))
		before := it.Clone()
		if err := it.Apply(action, clock); err != nil {
			t.Fatalf("%s at %s: %v", action, steps[i+1], err)
		}
		reason := ReasonUser
		if action == Reopen {
			reason = ReasonReopen
		}
		events = append(events, Record(&before, it, clock, clock, reason))
	}
	for i := range events {
		events[i].Seq = int64(i + 1)
	}
	return it, events
}

func TestRecordedEventsValidate(t *testing.T) {
	_, events := history(t, Start, "09:00:00", Pause, "09:10:00", Resume, "09:30:00", Finish, "09:35:00", Reopen, "10:00:00", Skip, "10:05:00", Unskip, "10:10:00")
	for _, e := range events {
		if err := e.Validate(); err != nil {
			t.Fatalf("%s to %s: %v", e.FromStatus, e.ToStatus, err)
		}
	}
}

func TestEditArchiveAndAutoPauseEventsValidate(t *testing.T) {
	it := newTestItem(t, "Study")
	_ = it.Apply(Start, at("09:00:00"))
	before := it.Clone()
	_ = it.Edit("Deep study", 90, at("09:05:00"))
	if err := Record(&before, it, at("09:05:00"), at("09:05:00"), ReasonUser).Validate(); err != nil {
		t.Fatal("edit:", err)
	}
	before = it.Clone()
	_ = it.Apply(Pause, at("09:06:00"))
	if err := Record(&before, it, at("09:06:00"), at("09:06:00"), ReasonAutoPause).Validate(); err != nil {
		t.Fatal("auto pause:", err)
	}
	before = it.Clone()
	_ = it.Archive(at("09:07:00"))
	if err := Record(&before, it, at("09:07:00"), at("09:07:00"), ReasonUser).Validate(); err != nil {
		t.Fatal("archive:", err)
	}
}

func TestDayEndEventValidates(t *testing.T) {
	it := newTestItem(t, "Study")
	_ = it.Apply(Start, at("09:00:00"))
	before := it.Clone()
	cutoff, noticed := at("23:59:59").Add(time.Second), at("23:59:59").Add(2*time.Hour)
	it.Status, it.ClosedAt, it.UpdatedAt = Incomplete, &cutoff, &noticed
	if err := Record(&before, it, cutoff, noticed, ReasonDayEnd).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestForgedEventsAreRefused(t *testing.T) {
	it := newTestItem(t, "Study")
	_ = it.Apply(Start, at("09:00:00"))
	started := it.Clone()
	later := at("09:30:00")

	fresh := newTestItem(t, "Study")
	jumped := fresh.Clone()
	jumped.Status, jumped.StartedAt, jumped.FinishedAt, jumped.ClosedAt, jumped.UpdatedAt = Done, &later, &later, &later, &later
	movedStart := started.Clone()
	_ = movedStart.Apply(Pause, later)
	moved := later.Add(-time.Hour)
	movedStart.StartedAt = &moved
	todoAgain := started.Clone()
	todoAgain.Status, todoAgain.StartedAt, todoAgain.UpdatedAt = Todo, nil, &later
	paused := started.Clone()
	_ = paused.Apply(Pause, later)

	cases := map[string]Event{
		"finishing without starting": Record(&fresh, jumped, later, later, ReasonUser),
		"rewriting the start":        Record(&started, movedStart, later, later, ReasonUser),
		"going back to todo":         Record(&started, todoAgain, later, later, ReasonUser),
		"wrong reason":               Record(&started, paused, later, later, ReasonReopen),
		"recorded at other time":     Record(&started, paused, later, later.Add(time.Second), ReasonUser),
		"unknown reason":             Record(&started, paused, later, later, "magic"),
	}
	for name, e := range cases {
		if err := e.Validate(); !errors.Is(err, ErrInvalidEvidence) {
			t.Errorf("%s was accepted: %v", name, err)
		}
	}
	created := newTestItem(t, "Study")
	created.Status = Done
	if err := Record(nil, created, created.CreatedAt, created.CreatedAt, ReasonUser).Validate(); !errors.Is(err, ErrInvalidEvidence) {
		t.Error("a creation event must create a todo item")
	}
}

func TestRecordDetachesTheItems(t *testing.T) {
	it := newTestItem(t, "Study")
	_ = it.Apply(Start, at("09:00:00"))
	e := Record(nil, it, at("09:00:00"), at("09:00:00"), ReasonUser)
	*it.StartedAt = at("12:00:00")
	if !e.After.StartedAt.Equal(at("09:00:00")) {
		t.Fatal("changing the item later changed the recorded event")
	}
}
