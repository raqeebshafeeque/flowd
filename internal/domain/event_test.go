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
		if err := it.Apply(action, clock, testDay); err != nil {
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
	_ = it.Apply(Start, at("09:00:00"), testDay)
	before := it.Clone()
	_ = it.Edit("Deep study", 90, at("09:05:00"))
	if err := Record(&before, it, at("09:05:00"), at("09:05:00"), ReasonUser).Validate(); err != nil {
		t.Fatal("edit:", err)
	}
	before = it.Clone()
	_ = it.Apply(Pause, at("09:06:00"), testDay)
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
	_ = it.Apply(Start, at("09:00:00"), testDay)
	before := it.Clone()
	cutoff, noticed := at("23:59:59").Add(time.Second), at("23:59:59").Add(2*time.Hour)
	it.Status, it.ClosedAt, it.UpdatedAt = Incomplete, &cutoff, &noticed
	if err := Record(&before, it, cutoff, noticed, ReasonDayEnd).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEventsAreCheckedAgainstTheirDay(t *testing.T) {
	_, events := history(t, Start, "09:00:00")
	if err := events[1].ValidateFor(testDay); err != nil {
		t.Fatal(err)
	}
	other := Day{ID: "dy_other", Date: "2026-11-17", Timezone: "UTC"}
	if err := events[1].ValidateFor(other); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal("an event from another day must be refused", err)
	}
	yesterday := Day{ID: "dy_test", Date: "2026-11-16", Timezone: "UTC"}
	if err := events[1].ValidateFor(yesterday); !errors.Is(err, ErrDayClosed) {
		t.Fatal("a start after the day closed must be refused", err)
	}
	it := newTestItem(t, "Study")
	_ = it.Apply(Start, at("09:00:00"), testDay)
	before := it.Clone()
	early := at("23:00:00")
	_, _ = it.CloseDay(testDay, at("23:59:59").Add(time.Second))
	moved := Record(&before, it, early, at("23:59:59").Add(time.Second), ReasonDayEnd)
	moved.After.ClosedAt = &early
	if err := moved.ValidateFor(testDay); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal("a day end before midnight must be refused", err)
	}
}

func TestForgedEventsAreRefused(t *testing.T) {
	it := newTestItem(t, "Study")
	_ = it.Apply(Start, at("09:00:00"), testDay)
	started := it.Clone()
	later := at("09:30:00")

	fresh := newTestItem(t, "Study")
	jumped := fresh.Clone()
	jumped.Status, jumped.StartedAt, jumped.FinishedAt, jumped.ClosedAt, jumped.UpdatedAt = Done, &later, &later, &later, &later
	movedStart := started.Clone()
	_ = movedStart.Apply(Pause, later, testDay)
	moved := later.Add(-time.Hour)
	movedStart.StartedAt = &moved
	todoAgain := started.Clone()
	todoAgain.Status, todoAgain.StartedAt, todoAgain.UpdatedAt = Todo, nil, &later
	paused := started.Clone()
	_ = paused.Apply(Pause, later, testDay)

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
	_ = it.Apply(Start, at("09:00:00"), testDay)
	e := Record(nil, it, at("09:00:00"), at("09:00:00"), ReasonUser)
	*it.StartedAt = at("12:00:00")
	if !e.After.StartedAt.Equal(at("09:00:00")) {
		t.Fatal("changing the item later changed the recorded event")
	}
}

// closed is an item on testDay that the day's end left incomplete, with its
// history.
func closed(t *testing.T) (Item, []Event) {
	t.Helper()
	it, events := history(t, Start, "09:00:00", Pause, "09:30:00")
	before := it.Clone()
	cutoff := at("23:59:59").Add(time.Second)
	if _, err := it.CloseDay(testDay, cutoff); err != nil {
		t.Fatal(err)
	}
	events = append(events, Record(&before, it, cutoff, cutoff, ReasonDayEnd))
	events[len(events)-1].Seq = int64(len(events))
	return it, events
}

// Marking a past item done is the one change accepted after its day ended,
// and only when nothing but the status changes.
func TestACorrectionIsTheOnlyChangeAfterTheDay(t *testing.T) {
	it, events := closed(t)
	later := at("23:59:59").Add(10 * time.Hour)
	before := it.Clone()
	if err := it.CorrectToDone(later); err != nil {
		t.Fatal(err)
	}
	good := Record(&before, it, later, later, ReasonUser)
	if err := good.ValidateFor(testDay); err != nil {
		t.Fatalf("a real correction must be accepted: %v", err)
	}
	good.Seq = int64(len(events) + 1)
	m, err := Measure(testDay, it, append(events, good), later)
	if err != nil || m.WorkNS != int64(30*time.Minute) {
		t.Fatalf("the work stays what was tracked: %+v, %v", m, err)
	}
	forged := it.Clone()
	forged.Title = "Something else"
	if err := Record(&before, forged, later, later, ReasonUser).ValidateFor(testDay); err == nil {
		t.Fatal("a correction that changes anything else must be refused")
	}
	early := it.Clone()
	stamp := at("12:00:00")
	early.CorrectedAt, early.UpdatedAt = &stamp, &stamp
	if err := Record(&before, early, stamp, stamp, ReasonUser).ValidateFor(testDay); err == nil {
		t.Fatal("a correction before the day ended must be refused")
	}
	if err := it.CorrectToDone(later.Add(time.Hour)); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("an item is corrected once: %v", err)
	}
}
