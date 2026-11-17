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

func addDuration(f *fixture, selection Selection, duration string) (Result, error) {
	return f.client.Do(ctx, Intent{Operation: Add, Selection: selection, Title: text("Task"), Duration: text(duration)})
}

func TestADurationCannotBeLongerThanWhatIsLeftToday(t *testing.T) {
	f := newFixture(t)
	f.at("20:00:00") // four hours to midnight
	_, err := addDuration(f, Selection{}, "30h")
	if !errors.Is(err, ErrDurationExceedsDay) || !errors.Is(err, domain.ErrInvalidDuration) {
		t.Fatalf("30h was not refused as too long: %v", err)
	}
	if fb := Describe(err); fb.Code != "ERR_DURATION_LIMIT" || fb.Exit != 2 || fb.Summary != "Only 4h left on this day" {
		t.Fatalf("got %+v", fb)
	}
	if _, err := addDuration(f, Selection{}, "4h"); err != nil {
		t.Fatal("exactly the time left must be accepted:", err)
	}
	_, err = addDuration(f, Selection{}, "1m")
	if fb := Describe(err); fb.Summary != "No time left on this day" {
		t.Fatalf("a full day should say so: %+v", fb)
	}
	if state := f.read(Selection{}); len(state.View.Items) != 1 {
		t.Fatal("a refused add saved something")
	}
}

func TestOnlyUnfinishedWorkHoldsTime(t *testing.T) {
	f := newFixture(t)
	f.at("20:00:00")
	done, _ := addDuration(f, Selection{}, "2h")
	skipped, _ := addDuration(f, Selection{}, "1h")
	if _, err := addDuration(f, Selection{}, "1h"); err != nil {
		t.Fatal(err)
	}
	if _, err := addDuration(f, Selection{}, "1m"); err == nil {
		t.Fatal("the day is full")
	}
	f.must(Start, done.Item.ID)
	f.must(Finish, done.Item.ID)
	f.must(Skip, skipped.Item.ID)
	if _, err := addDuration(f, Selection{}, "3h"); err != nil {
		t.Fatal("done and skipped items must free their time:", err)
	}
}

func TestARunningItemHoldsOnlyWhatIsNotWorkedYet(t *testing.T) {
	f := newFixture(t)
	f.at("20:00:00")
	r, _ := addDuration(f, Selection{}, "2h")
	f.must(Start, r.Item.ID)
	f.at("21:00:00") // three hours left, one worked, one still held
	if _, err := addDuration(f, Selection{}, "2h1m"); !errors.Is(err, ErrDurationExceedsDay) {
		t.Fatal("the unworked hour must still be held", err)
	}
	if _, err := addDuration(f, Selection{}, "2h"); err != nil {
		t.Fatal(err)
	}
}

func TestFutureDaysHaveTheWholeDay(t *testing.T) {
	f := newFixture(t)
	later := Selection{Date: "2026-11-20"}
	if _, err := addDuration(f, later, "24h1m"); !errors.Is(err, ErrDurationExceedsDay) {
		t.Fatal("more than a day was accepted", err)
	}
	if _, err := addDuration(f, later, "20h"); err != nil {
		t.Fatal(err)
	}
	if _, err := addDuration(f, later, "4h1m"); !errors.Is(err, ErrDurationExceedsDay) {
		t.Fatal("the future day ignored its other items", err)
	}
}

func TestEditsAreCheckedOnlyWhenTheyGrow(t *testing.T) {
	f := newFixture(t)
	f.at("20:00:00")
	a, _ := addDuration(f, Selection{}, "1h")
	addDuration(f, Selection{}, "1h") // two hours free for A to grow into
	grow := func(d string) error {
		_, err := f.change(Edit, a.Item.ID, func(i *Intent) { i.Duration = text(d) })
		return err
	}
	if err := grow("3h1m"); !errors.Is(err, ErrDurationExceedsDay) {
		t.Fatal("growing past the time left was accepted", err)
	}
	if err := grow("3h"); err != nil {
		t.Fatal("an item must not count against itself:", err)
	}
	if err := grow("30m"); err != nil {
		t.Fatal("shortening must always work:", err)
	}
}

func TestAnOverPlannedDayCanStillBeRenamed(t *testing.T) {
	f := newFixture(t)
	r, _ := addDuration(f, Selection{}, "10h")
	f.at("20:00:00") // the 10h item now holds more than is left
	if _, err := f.change(Edit, r.Item.ID, func(i *Intent) { i.Title = text("Renamed") }); err != nil {
		t.Fatal("a title only edit must work on a full day:", err)
	}
	if _, err := f.change(Edit, r.Item.ID, func(i *Intent) { i.Duration = text("9h") }); err != nil {
		t.Fatal("shortening must work on a full day:", err)
	}
}

func TestBadDurationTextIsStillAFormatError(t *testing.T) {
	f := newFixture(t)
	_, err := addDuration(f, Selection{}, "lots")
	if errors.Is(err, ErrDurationExceedsDay) || Describe(err).Code != "ERR_DURATION" {
		t.Fatal("a malformed duration must keep its own error", err)
	}
}
