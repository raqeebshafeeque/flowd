package planner

import (
	"errors"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// on moves the fixture's clock to 09:00 on a day of November 2026.
func (f *fixture) on(day int) { f.now = time.Date(2026, 11, day, 9, 0, 0, 0, time.UTC) }

// leftoverWeek plans earlier days and comes back to the 17th: two items left
// incomplete on the 15th, one left incomplete on the 16th,
// one finished on the 16th and one left too long ago, on the 9th.
func leftoverWeek(t *testing.T) (f *fixture, a, b, c, old domain.ID) {
	f = newFixture(t)
	f.on(9)
	old = f.add("Too old", "1h").Item.ID
	f.on(15)
	a = f.add("Write release notes", "1h").Item.ID
	b = f.add("Reply to the review", "30m").Item.ID
	f.on(16)
	c = f.add("Refactor the scheduler", "2h").Item.ID
	done := f.add("Inbox", "20m").Item.ID
	f.must(Start, done)
	f.must(Finish, done)
	f.on(17)
	return f, a, b, c, old
}

func (f *fixture) carry(date string, ids ...domain.ID) (Result, error) {
	return f.client.Do(ctx, Intent{Operation: Carry, Selection: Selection{Date: date}, Sources: ids})
}

func titles(items []Leftover) []string {
	var out []string
	for _, l := range items {
		out = append(out, l.Date+" "+l.Item.Title)
	}
	return out
}

func TestOnlyIncompleteItemsFromTheLastWeekAreOffered(t *testing.T) {
	f, _, _, _, _ := leftoverWeek(t)
	got, err := f.client.Leftovers(ctx, Selection{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-11-16 Refactor the scheduler", "2026-11-15 Write release notes", "2026-11-15 Reply to the review"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
	for i := range want {
		if titles(got)[i] != want[i] {
			t.Fatalf("got %v, want %v", titles(got), want)
		}
	}
}

// A carried item is a fresh todo copy with the same title and duration, and
// the original is left exactly as it was.
func TestCarryingCopiesTheItemsOntoToday(t *testing.T) {
	f, a, b, c, _ := leftoverWeek(t)
	before := f.read(Selection{Date: "2026-11-15"}).View.Items
	r, err := f.carry("", b, c)
	if err != nil || r.Outcome != Confirmed || len(r.Carried) != 2 {
		t.Fatalf("got %+v, %v", r, err)
	}
	today := f.read(Selection{}).View.Items
	if len(today) != 2 || today[0].Title != "Reply to the review" || today[1].Title != "Refactor the scheduler" {
		t.Fatalf("today %+v", today)
	}
	for i, from := range []domain.ID{b, c} {
		copied := today[i]
		if copied.ID == from || copied.CarriedFrom == nil || *copied.CarriedFrom != from || copied.Status != domain.Todo || copied.WorkNS != 0 || copied.StartedAt != nil {
			t.Fatalf("copy %d: %+v", i, copied.Item)
		}
	}
	if today[0].DurationMinutes != 30 || today[1].DurationMinutes != 120 {
		t.Fatal("a copy keeps the original duration")
	}
	after := f.read(Selection{Date: "2026-11-15"}).View.Items
	for i := range before {
		if !domain.Same(before[i].Item, after[i].Item) {
			t.Fatalf("the original changed: %+v", after[i].Item)
		}
	}
	left, _ := f.client.Leftovers(ctx, Selection{})
	if len(left) != 1 || left[0].Item.ID != a {
		t.Fatalf("carried items are not offered again: %v", titles(left))
	}
}

// An item is carried once. Deleting its copy frees it again.
func TestAnItemIsCarriedOnlyOnce(t *testing.T) {
	f, _, b, _, _ := leftoverWeek(t)
	r, err := f.carry("", b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.carry("", b); !errors.Is(err, ErrNotCarryable) || Describe(err).Summary != "That item was carried already" {
		t.Fatalf("got %v", err)
	}
	f.must(Delete, r.Item.ID)
	if _, err := f.carry("", b); err != nil {
		t.Fatalf("a deleted copy should free the item: %v", err)
	}
}

func TestOnlyTheLastSevenDaysCanBeCarriedFrom(t *testing.T) {
	f, _, _, _, old := leftoverWeek(t)
	_, err := f.carry("", old)
	if !errors.Is(err, ErrNotCarryable) || Describe(err).Summary != "Only items from the 7 days before 2026-11-17 can be carried" {
		t.Fatalf("got %v", err)
	}
	// Carrying onto a later day reaches back from that day.
	if _, err := f.carry("2026-11-16", old); !errors.Is(err, ErrPastDay) {
		t.Fatalf("a past day cannot be carried onto: %v", err)
	}
}

func TestOnlyIncompleteItemsCanBeCarried(t *testing.T) {
	f, _, _, _, _ := leftoverWeek(t)
	done := f.read(Selection{Date: "2026-11-16"}).View.Items
	var inbox domain.ID
	for _, it := range done {
		if it.Title == "Inbox" {
			inbox = it.ID
		}
	}
	if _, err := f.carry("", inbox); !errors.Is(err, ErrNotCarryable) || Describe(err).Code != "ERR_NOT_CARRYABLE" {
		t.Fatalf("got %v", err)
	}
	if _, err := f.carry(""); Describe(err).Code != "ERR_USAGE" {
		t.Fatalf("nothing picked: %v", err)
	}
}

// When the picked items do not all fit in what is left of the day, nothing
// is carried.
func TestCarryingIsAllOrNothing(t *testing.T) {
	f := newFixture(t)
	f.on(15)
	big := f.add("Big", "10h").Item.ID
	f.on(16)
	bigger := f.add("Bigger", "8h").Item.ID
	f.on(17)
	_, err := f.carry("", big, bigger)
	if !errors.Is(err, ErrDurationExceedsDay) || Describe(err).Summary != "The 2 picked items need 18h, and only 15h is left on 2026-11-17" {
		t.Fatalf("got %v", err)
	}
	if items := f.read(Selection{}).View.Items; len(items) != 0 {
		t.Fatalf("nothing should be carried: %+v", items)
	}
	if _, err := f.carry("", big); err != nil {
		t.Fatalf("one fits: %v", err)
	}
}

// Carrying onto a later day works, and reaches back seven days from it.
func TestCarryingOntoALaterDay(t *testing.T) {
	f, a, _, _, _ := leftoverWeek(t)
	if _, err := f.carry("2026-11-20", a); err != nil {
		t.Fatal(err)
	}
	if items := f.read(Selection{Date: "2026-11-20"}).View.Items; len(items) != 1 || items[0].Title != "Write release notes" {
		t.Fatalf("got %+v", items)
	}
}
