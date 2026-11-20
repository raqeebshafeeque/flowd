package planner

import (
	"errors"
	"strings"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

func (f *fixture) titles(date string) string {
	f.t.Helper()
	var names []string
	for _, it := range f.read(Selection{Date: date}).View.Items {
		names = append(names, it.Title)
	}
	return strings.Join(names, " ")
}

func (f *fixture) reorder(ids ...domain.ID) (Result, error) {
	return f.client.Do(ctx, Intent{Operation: Reorder, Order: ids})
}

func TestTheDayIsInTheOrderYouSave(t *testing.T) {
	f := newFixture(t)
	a, b, c := f.add("A", "1h").Item.ID, f.add("B", "1h").Item.ID, f.add("C", "1h").Item.ID
	r, err := f.reorder(c, a, b)
	if err != nil || r.Outcome != Confirmed {
		t.Fatalf("got %+v, %v", r, err)
	}
	if got := f.titles(""); got != "C A B" {
		t.Fatalf("got %s", got)
	}
	// Changing status never moves anything.
	f.must(Start, b)
	f.must(Finish, b)
	f.must(Start, a)
	if got := f.titles(""); got != "C A B" {
		t.Fatalf("a status change moved an item: %s", got)
	}
}

// If the day changed since it was read, nothing is saved.
func TestAnOrderFromBeforeAChangeIsRefused(t *testing.T) {
	f := newFixture(t)
	a, b := f.add("A", "1h").Item.ID, f.add("B", "1h").Item.ID
	f.add("C", "1h") // added while A and B were being reordered
	_, err := f.reorder(b, a)
	if !errors.Is(err, store.ErrOrderChanged) || Describe(err).Code != "ERR_STALE" || Describe(err).Summary != "The day changed while it was being reordered" {
		t.Fatalf("got %v", err)
	}
	if got := f.titles(""); got != "A B C" {
		t.Fatalf("nothing should change: %s", got)
	}
}

func TestAPastDayCannotBeReordered(t *testing.T) {
	f := newFixture(t)
	f.on(16)
	a, b := f.add("A", "1h").Item.ID, f.add("B", "1h").Item.ID
	f.on(17)
	if _, err := f.client.Do(ctx, Intent{Operation: Reorder, Selection: Selection{Date: "2026-11-16"}, Order: []domain.ID{b, a}}); !errors.Is(err, ErrPastDay) {
		t.Fatalf("got %v", err)
	}
}

// Carried items join the end of the day, in the order they were picked.
func TestCarriedItemsJoinTheEnd(t *testing.T) {
	f, a, b, _, _ := leftoverWeek(t)
	f.add("Today", "30m")
	if _, err := f.carry("", b, a); err != nil {
		t.Fatal(err)
	}
	if got := f.titles(""); got != "Today Reply to the review Write release notes" {
		t.Fatalf("got %s", got)
	}
}
