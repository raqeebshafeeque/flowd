package planner

import (
	"errors"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func (f *fixture) markDone(date string, id domain.ID) (Result, error) {
	f.t.Helper()
	state := f.read(Selection{Date: date})
	return f.client.Do(ctx, Intent{Operation: MarkDone, Selection: Selection{Date: date}, ItemID: id, Revision: state.Revisions[id]})
}

func (f *fixture) item(date string, id domain.ID) ItemView {
	f.t.Helper()
	for _, it := range f.read(Selection{Date: date}).View.Items {
		if it.ID == id {
			return it
		}
	}
	f.t.Fatalf("no item %s on %s", id, date)
	return ItemView{}
}

// A forgotten finish is put right: the item becomes done, keeps the work it
// really had, and says it was marked done later.
func TestAForgottenFinishIsPutRight(t *testing.T) {
	f := newFixture(t)
	f.on(16)
	worked := f.add("Worked on", "1h").Item.ID
	never := f.add("Never started", "30m").Item.ID
	f.must(Start, worked)
	f.now = f.now.Add(40 * time.Minute)
	f.must(Pause, worked)
	f.on(17)
	for _, id := range []domain.ID{worked, never} {
		if _, err := f.markDone("2026-11-16", id); err != nil {
			t.Fatal(err)
		}
	}
	w, n := f.item("2026-11-16", worked), f.item("2026-11-16", never)
	if w.Status != domain.Done || w.CorrectedAt == nil || !w.CorrectedAt.Equal(f.now) || w.WorkNS != int64(40*time.Minute) {
		t.Fatalf("worked: %+v", w)
	}
	if n.Status != domain.Done || n.StartedAt != nil || n.WorkNS != 0 {
		t.Fatalf("no work or start is invented: %+v", n)
	}
	midnight := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	if !w.FinishedAt.Equal(midnight) {
		t.Fatalf("an incomplete item finishes when its day closed, got %v", w.FinishedAt)
	}
}

// Only past days, within the last 7, and only items left incomplete, once.
func TestOnlyRecentUnfinishedPastItemsArePutRight(t *testing.T) {
	f := newFixture(t)
	f.on(9)
	old := f.add("Too old", "30m").Item.ID
	f.on(16)
	done := f.add("Done at the time", "30m").Item.ID
	f.must(Start, done)
	f.must(Finish, done)
	left := f.add("Left", "30m").Item.ID
	f.on(17)
	today := f.add("Today", "30m").Item.ID
	if _, err := f.markDone("", today); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Fatalf("today: %v", err)
	}
	if _, err := f.markDone("2026-11-16", done); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Fatalf("already done: %v", err)
	}
	if _, err := f.markDone("2026-11-09", old); !errors.Is(err, ErrPastDay) {
		t.Fatalf("too old: %v", err)
	}
	if _, err := f.markDone("2026-11-16", left); err != nil {
		t.Fatal(err)
	}
	if _, err := f.markDone("2026-11-16", left); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Fatalf("twice: %v", err)
	}
}

// Once it is done it is no longer offered to carry.
func TestAMarkedDoneItemIsNotCarried(t *testing.T) {
	f, a, _, _, _ := leftoverWeek(t)
	if _, err := f.markDone("2026-11-15", a); err != nil {
		t.Fatal(err)
	}
	left, _ := f.client.Leftovers(ctx, Selection{})
	for _, l := range left {
		if l.Item.ID == a {
			t.Fatal("a done item cannot be carried")
		}
	}
}
