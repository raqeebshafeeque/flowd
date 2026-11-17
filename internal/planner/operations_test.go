package planner

import (
	"errors"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func TestAddCreatesTheDayAndItem(t *testing.T) {
	f := newFixture(t)
	r := f.add("Study", "1h30m")
	if r.Outcome != Confirmed || r.Item.DurationMinutes != 90 || r.Item.Status != domain.Todo || r.Day.ID == "" {
		t.Fatalf("unexpected result: %+v", r)
	}
	state := f.read(Selection{})
	if len(state.View.Items) != 1 || state.View.Items[0].Title != "Study" {
		t.Fatalf("not listed: %+v", state.View.Items)
	}
}

func TestBadInputIsRefusedWithoutSaving(t *testing.T) {
	f := newFixture(t)
	cases := map[string]Intent{
		"bad duration":  {Operation: Add, Title: text("Study"), Duration: text("1.5h")},
		"empty title":   {Operation: Add, Title: text(" "), Duration: text("1h")},
		"missing title": {Operation: Add, Duration: text("1h")},
		"unknown":       {Operation: "dance"},
	}
	for name, intent := range cases {
		r, err := f.client.Do(ctx, intent)
		if err == nil || r.Outcome == Confirmed || r.Outcome == Unconfirmed {
			t.Errorf("%s: got %+v, %v", name, r, err)
		}
	}
	if state := f.read(Selection{}); state.Day.ID != "" {
		t.Fatal("a refused add created a day")
	}
}

func TestPastDaysCannotBePlanned(t *testing.T) {
	f := newFixture(t)
	for _, date := range []string{"2026-11-16", "2020-01-01"} {
		_, err := f.client.Do(ctx, Intent{Operation: Add, Selection: Selection{Date: date}, Title: text("Late"), Duration: text("1h")})
		if !errors.Is(err, ErrPastDay) {
			t.Errorf("%s: got %v", date, err)
		}
	}
	if _, err := f.client.Do(ctx, Intent{Operation: Add, Selection: Selection{Date: "2026-11-18"}, Title: text("Tomorrow"), Duration: text("1h")}); err != nil {
		t.Fatal("tomorrow must be plannable:", err)
	}
}

func TestPlanningTodayFollowsThisComputersZone(t *testing.T) {
	f := newFixture(t)
	f.client.Zone, _ = time.LoadLocation("Asia/Kolkata")
	f.at("20:00:00") // already 18 November in India
	_, err := f.client.Do(ctx, Intent{Operation: Add, Selection: Selection{Date: "2026-11-17", Timezone: "UTC"}, Title: text("Sneaky"), Duration: text("1h")})
	if !errors.Is(err, ErrPastDay) {
		t.Fatal("choosing a different timezone must not reopen yesterday", err)
	}
}

func TestEditAndDelete(t *testing.T) {
	f := newFixture(t)
	item := f.add("Study", "1h").Item
	r, err := f.change(Edit, item.ID, func(i *Intent) { i.Title, i.Duration = text("Deep study"), text("2h") })
	if err != nil || r.Item.Title != "Deep study" || r.Item.DurationMinutes != 120 {
		t.Fatalf("edit: %+v, %v", r.Item, err)
	}
	if _, err := f.change(Edit, item.ID); !errors.Is(err, ErrInput) {
		t.Fatal("an edit with nothing to change must be refused", err)
	}
	if _, err := f.change(Delete, item.ID); err != nil {
		t.Fatal(err)
	}
	if state := f.read(Selection{}); len(state.View.Items) != 0 {
		t.Fatal("a deleted item is still listed")
	}
	if _, err := f.client.Do(ctx, Intent{Operation: Start, ItemID: item.ID, Revision: r.Revisions[item.ID]}); err == nil {
		t.Fatal("a deleted item must not start")
	}
}

func TestStartingAnotherItemPausesTheRunningOne(t *testing.T) {
	f := newFixture(t)
	a := f.add("A", "1h").Item
	b := f.add("B", "1h").Item
	f.must(Start, a.ID)
	f.at("09:40:00")
	r := f.must(Start, b.ID)
	if len(r.AutoPaused) != 1 || r.AutoPaused[0] != a.ID {
		t.Fatalf("auto paused: %v", r.AutoPaused)
	}
	state := f.read(Selection{})
	if state.View.Running == nil || state.View.Running.ID != b.ID {
		t.Fatal("B is not the running item")
	}
	for _, item := range state.View.Items {
		if item.ID == a.ID && (item.Status != domain.Paused || item.WorkNS != int64(40*time.Minute)) {
			t.Fatalf("A was not paused at the switch: %+v", item)
		}
	}
	events := state.History[a.ID]
	if last := events[len(events)-1]; last.Reason != domain.ReasonAutoPause {
		t.Fatalf("A's pause reason is %s", last.Reason)
	}
}

func TestTrackingOnlyWorksToday(t *testing.T) {
	f := newFixture(t)
	r, err := f.client.Do(ctx, Intent{Operation: Add, Selection: Selection{Date: "2026-11-18"}, Title: text("Tomorrow"), Duration: text("1h")})
	if err != nil {
		t.Fatal(err)
	}
	state := f.read(Selection{Date: "2026-11-18"})
	_, err = f.client.Do(ctx, Intent{Operation: Start, Selection: Selection{Date: "2026-11-18"}, ItemID: r.Item.ID, Revision: state.Revisions[r.Item.ID]})
	if !errors.Is(err, domain.ErrDayClosed) {
		t.Fatal("starting tomorrow's item today must be refused", err)
	}
}

func TestMidnightClosesTheDayOnTheNextOperation(t *testing.T) {
	f := newFixture(t)
	item := f.add("Study", "1h").Item
	f.must(Start, item.ID)
	f.now = time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	state := f.read(Selection{Date: "2026-11-17"})
	got := state.View.Items[0]
	if got.Status != domain.Incomplete || got.WorkNS != int64(15*time.Hour) {
		t.Fatalf("not closed at midnight: %s with %v work", got.Status, time.Duration(got.WorkNS))
	}
	if today := f.read(Selection{}); len(today.View.Items) != 0 {
		t.Fatal("nothing should carry over to the next day")
	}
}

func TestEveryStatusActionWorksThroughThePlanner(t *testing.T) {
	f := newFixture(t)
	item := f.add("Study", "1h").Item
	steps := []struct {
		op   Operation
		want domain.Status
	}{{Start, domain.InProgress}, {Pause, domain.Paused}, {Resume, domain.InProgress}, {Finish, domain.Done}, {Reopen, domain.InProgress}, {Skip, domain.Skipped}, {Unskip, domain.Paused}}
	for i, s := range steps {
		f.now = f.now.Add(time.Duration(i+1) * time.Minute)
		if r := f.must(s.op, item.ID); r.Item.Status != s.want {
			t.Fatalf("%s gave %s", s.op, r.Item.Status)
		}
	}
	if _, err := f.change(Pause, item.ID); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Fatal("pausing a paused item must be refused", err)
	}
}
