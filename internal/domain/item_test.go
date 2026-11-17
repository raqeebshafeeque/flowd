package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// testDay is 2026-11-17 in UTC, the day every test item belongs to.
var testDay = Day{ID: "dy_test", Date: "2026-11-17", Timezone: "UTC"}

// at returns 2026-11-17 at the given clock time in UTC.
func at(clock string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", "2026-11-17 "+clock)
	if err != nil {
		panic(err)
	}
	return t
}

func newTestItem(t *testing.T, title string) Item {
	t.Helper()
	it, err := NewItem("dy_test", title, 60, at("08:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func TestNewItem(t *testing.T) {
	it, err := NewItem("dy_test", "Study", 90, at("08:00:00").Add(400*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if it.Status != Todo || it.DurationMinutes != 90 || !it.CreatedAt.Equal(at("08:00:00")) || it.StartedAt != nil || !strings.HasPrefix(string(it.ID), "it_") {
		t.Fatalf("unexpected new item: %+v", it)
	}
	if err := it.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewItem("dy_test", "Study", 0, at("08:00:00")); !errors.Is(err, ErrInvalidDuration) {
		t.Fatal("a zero duration must be refused", err)
	}
	if _, err := NewItem("dy_test", "Study", 30, time.Time{}); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal("a missing clock must be refused", err)
	}
}

func TestValidateTitle(t *testing.T) {
	for _, good := range []string{"Study", "Fix the login bug", strings.Repeat("a", 120), "界界 é 👩‍💻", "  padded  "} {
		if err := ValidateTitle(good); err != nil {
			t.Errorf("%q refused: %v", good, err)
		}
	}
	for _, bad := range []string{"", "   ", strings.Repeat("a", 121), "two\nlines", "carriage\rreturn", string([]byte{0xff})} {
		if err := ValidateTitle(bad); !errors.Is(err, ErrInvalidTitle) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestEveryAllowedAction(t *testing.T) {
	type step struct {
		action Action
		want   Status
	}
	cases := []struct {
		name  string
		steps []step
	}{
		{"start pause resume finish", []step{{Start, InProgress}, {Pause, Paused}, {Resume, InProgress}, {Finish, Done}}},
		{"finish while paused", []step{{Start, InProgress}, {Pause, Paused}, {Finish, Done}}},
		{"skip before starting then unskip", []step{{Skip, Skipped}, {Unskip, Todo}}},
		{"skip after starting then unskip", []step{{Start, InProgress}, {Skip, Skipped}, {Unskip, Paused}}},
		{"skip while paused", []step{{Start, InProgress}, {Pause, Paused}, {Skip, Skipped}}},
		{"reopen a finished item", []step{{Start, InProgress}, {Finish, Done}, {Reopen, InProgress}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := newTestItem(t, "Study")
			now := at("09:00:00")
			for _, s := range c.steps {
				now = now.Add(time.Minute)
				if err := it.Apply(s.action, now, testDay); err != nil {
					t.Fatalf("%s: %v", s.action, err)
				}
				if it.Status != s.want {
					t.Fatalf("%s gave %s, want %s", s.action, it.Status, s.want)
				}
				if err := it.Check(); err != nil {
					t.Fatalf("%s left the item inconsistent: %v", s.action, err)
				}
			}
		})
	}
}

func TestRefusedActionsChangeNothing(t *testing.T) {
	refused := map[Status][]Action{
		Todo:       {Pause, Resume, Finish, Unskip, Reopen},
		InProgress: {Start, Resume, Unskip, Reopen},
		Paused:     {Start, Pause, Unskip, Reopen},
		Done:       {Start, Pause, Resume, Finish, Skip, Unskip},
		Skipped:    {Start, Pause, Resume, Finish, Skip, Reopen},
	}
	reach := map[Status][]Action{Todo: nil, InProgress: {Start}, Paused: {Start, Pause}, Done: {Start, Finish}, Skipped: {Skip}}
	for status, actions := range refused {
		for _, a := range actions {
			it := newTestItem(t, "Study")
			for i, step := range reach[status] {
				if err := it.Apply(step, at("09:00:00").Add(time.Duration(i)*time.Minute), testDay); err != nil {
					t.Fatal(err)
				}
			}
			before := it.Clone()
			if err := it.Apply(a, at("10:00:00"), testDay); !errors.Is(err, ErrIllegalTransition) {
				t.Errorf("%s on %s: got %v", a, status, err)
			}
			if !Same(before, it) {
				t.Errorf("%s on %s changed the item", a, status)
			}
		}
	}
}

func TestFirstStartNeverChanges(t *testing.T) {
	it := newTestItem(t, "Study")
	steps := []Action{Start, Pause, Resume, Finish, Reopen, Skip, Unskip, Resume}
	for i, a := range steps {
		if err := it.Apply(a, at("09:00:00").Add(time.Duration(i)*time.Minute), testDay); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		if !it.StartedAt.Equal(at("09:00:00")) {
			t.Fatalf("after %s the first start moved to %v", a, it.StartedAt)
		}
	}
}

func TestReopenClearsTheFinish(t *testing.T) {
	it := newTestItem(t, "Study")
	_ = it.Apply(Start, at("09:00:00"), testDay)
	_ = it.Apply(Finish, at("09:30:00"), testDay)
	if err := it.Apply(Reopen, at("10:00:00"), testDay); err != nil {
		t.Fatal(err)
	}
	if it.FinishedAt != nil || it.ClosedAt != nil {
		t.Fatalf("reopen kept the finish: %+v", it)
	}
}

func TestClosedItemsRefuseEveryAction(t *testing.T) {
	archived := newTestItem(t, "Study")
	if err := archived.Archive(at("09:00:00")); err != nil {
		t.Fatal(err)
	}
	incomplete := newTestItem(t, "Study")
	closed := at("23:59:59")
	incomplete.Status, incomplete.ClosedAt = Incomplete, &closed
	for _, it := range []Item{archived, incomplete} {
		for _, a := range []Action{Start, Skip, Unskip, Reopen} {
			copy := it.Clone()
			if err := copy.Apply(a, at("10:00:00"), testDay); !errors.Is(err, ErrIllegalTransition) {
				t.Errorf("%s on a closed item: %v", a, err)
			}
		}
	}
}

func TestActionsNeedAnOpenDay(t *testing.T) {
	it := newTestItem(t, "Study")
	tomorrow := at("23:59:59").Add(time.Second)
	if err := it.Apply(Start, tomorrow, testDay); !errors.Is(err, ErrDayClosed) {
		t.Fatal("starting at midnight must be refused", err)
	}
	future := Day{ID: "dy_test", Date: "2026-11-18", Timezone: "UTC"}
	if err := it.Apply(Start, at("09:00:00"), future); !errors.Is(err, ErrDayClosed) {
		t.Fatal("starting before the day opens must be refused", err)
	}
	if it.Status != Todo {
		t.Fatal("a refused start changed the item")
	}
}

func TestCloseDayMakesUnfinishedItemsIncomplete(t *testing.T) {
	cutoff, noticed := at("23:59:59").Add(time.Second), at("23:59:59").Add(3*time.Hour)
	open := map[string][]Action{"todo": nil, "running": {Start}, "paused": {Start, Pause}}
	for name, steps := range open {
		it := newTestItem(t, "Study")
		for i, a := range steps {
			_ = it.Apply(a, at("09:00:00").Add(time.Duration(i)*time.Minute), testDay)
		}
		changed, err := it.CloseDay(testDay, noticed)
		if err != nil || !changed || it.Status != Incomplete || !it.ClosedAt.Equal(cutoff) || !it.UpdatedAt.Equal(noticed) || it.FinishedAt != nil {
			t.Errorf("%s: changed=%v err=%v item=%+v", name, changed, err, it)
		}
	}
	done := newTestItem(t, "Study")
	_ = done.Apply(Start, at("09:00:00"), testDay)
	_ = done.Apply(Finish, at("09:30:00"), testDay)
	if changed, _ := done.CloseDay(testDay, noticed); changed || done.Status != Done {
		t.Error("a done item must stay done")
	}
	early := newTestItem(t, "Study")
	if changed, _ := early.CloseDay(testDay, at("23:00:00")); changed {
		t.Error("an open day must not close")
	}
}

func TestBackwardsClockIsRefused(t *testing.T) {
	it := newTestItem(t, "Study")
	_ = it.Apply(Start, at("09:00:00"), testDay)
	if err := it.Apply(Pause, at("08:59:59"), testDay); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatal("a pause before the start must be refused", err)
	}
	if it.Status != InProgress {
		t.Fatal("the refused pause changed the item")
	}
}

func TestEditKeepsStatusTimesAndOrder(t *testing.T) {
	it := newTestItem(t, "Study")
	_ = it.Apply(Start, at("09:00:00"), testDay)
	before := it.Clone()
	if err := it.Edit("Deep study", 120, at("09:10:00")); err != nil {
		t.Fatal(err)
	}
	if it.Title != "Deep study" || it.DurationMinutes != 120 || it.Status != before.Status || !it.StartedAt.Equal(*before.StartedAt) {
		t.Fatalf("edit changed more than the title and duration: %+v", it)
	}
	if err := it.Edit("", 30, at("09:20:00")); !errors.Is(err, ErrInvalidTitle) {
		t.Fatal(err)
	}
	if err := it.Edit("Ok", -1, at("09:20:00")); !errors.Is(err, ErrInvalidDuration) {
		t.Fatal(err)
	}
}

func TestArchiveStopsARunningItem(t *testing.T) {
	it := newTestItem(t, "Study")
	_ = it.Apply(Start, at("09:00:00"), testDay)
	if err := it.Archive(at("09:30:00")); err != nil {
		t.Fatal(err)
	}
	if !it.Archived || it.Status != InProgress || !it.ClosedAt.Equal(at("09:30:00")) {
		t.Fatalf("archive must keep the status and close it: %+v", it)
	}
	if err := it.Archive(at("09:40:00")); err != nil || !it.ClosedAt.Equal(at("09:30:00")) {
		t.Fatal("archiving twice must change nothing")
	}
}

func TestCheckFindsInconsistentItems(t *testing.T) {
	started := at("09:00:00")
	earlier := at("07:00:00")
	cases := map[string]func(*Item){
		"todo with a start":     func(it *Item) { it.StartedAt = &started },
		"running without start": func(it *Item) { it.Status = InProgress },
		"done without finish":   func(it *Item) { it.Status, it.StartedAt = Done, &started },
		"skipped without close": func(it *Item) { it.Status = Skipped },
		"open with a close":     func(it *Item) { it.ClosedAt = &started },
		"time before creation":  func(it *Item) { it.UpdatedAt = &earlier },
		"unknown status":        func(it *Item) { it.Status = "lost" },
		"finish but not done":   func(it *Item) { it.FinishedAt = &started },
	}
	for name, breakIt := range cases {
		it := newTestItem(t, "Study")
		breakIt(&it)
		if err := it.Check(); !errors.Is(err, ErrInvalidEvidence) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestOnlyOneItemRunsPerDay(t *testing.T) {
	a, b := newTestItem(t, "A"), newTestItem(t, "B")
	_ = a.Apply(Start, at("09:00:00"), testDay)
	_ = b.Apply(Start, at("09:01:00"), testDay)
	if err := CheckDay([]Item{a, b}); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatal("two running items must be refused", err)
	}
	_ = a.Apply(Pause, at("09:01:00"), testDay)
	if err := CheckDay([]Item{a, b}); err != nil {
		t.Fatal(err)
	}
}

func TestStartingOrderWins(t *testing.T) {
	a, b, c := newTestItem(t, "A"), newTestItem(t, "B"), newTestItem(t, "C")
	_ = c.Apply(Start, at("09:00:00"), testDay)
	_ = c.Apply(Pause, at("09:05:00"), testDay)
	_ = b.Apply(Start, at("09:05:00"), testDay)
	_ = c.Apply(Finish, at("09:20:00"), testDay)
	items := []Item{a, b, c}
	Sort(items)
	if got := items[0].Title + items[1].Title + items[2].Title; got != "CBA" {
		t.Fatalf("got order %s, want CBA (started first, then not started)", got)
	}
}

func TestOrderBreaksTiesByCreationThenID(t *testing.T) {
	first, err := NewItem("dy", "First", 30, at("08:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	second, _ := NewItem("dy", "Second", 30, at("08:00:01"))
	sameA, sameB := first, first
	sameA.ID, sameB.ID = "it_a", "it_b"
	items := []Item{second, sameB, sameA}
	Sort(items)
	if items[0].ID != "it_a" || items[1].ID != "it_b" || items[2].Title != "Second" {
		t.Fatalf("unexpected order: %v %v %v", items[0].ID, items[1].ID, items[2].Title)
	}
}
