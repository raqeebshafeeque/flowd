package planner

import (
	"errors"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func TestConfirmedResultCarriesFreshState(t *testing.T) {
	f := newFixture(t)
	item := f.add("Study", "1h").Item
	f.at("09:30:00")
	r := f.must(Start, item.ID)
	if r.Outcome != Confirmed || r.Revisions[item.ID].Seq == 0 || len(r.History[item.ID]) != 2 || r.Metrics.WorkNS != 0 {
		t.Fatalf("unexpected result: %+v", r)
	}
	if r.Revisions[item.ID] != f.read(Selection{}).Revisions[item.ID] {
		t.Fatal("the returned revision does not match what is saved")
	}
}

func TestReconcileFindsASavedChange(t *testing.T) {
	f := newFixture(t)
	item := f.add("Study", "1h").Item
	r := f.must(Start, item.ID)
	pending := r
	pending.Outcome = Unconfirmed
	got, err := f.client.Reconcile(ctx, pending)
	if err != nil || got.Outcome != Confirmed || got.Item.Status != domain.InProgress {
		t.Fatalf("got %+v, %v", got.Outcome, err)
	}
}

func TestReconcileSeesTheChangeEvenAfterMidnightClosedTheItem(t *testing.T) {
	f := newFixture(t)
	item := f.add("Study", "1h").Item
	r := f.must(Start, item.ID)
	f.now = time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC)
	got, err := f.client.Reconcile(ctx, r)
	if err != nil || got.Outcome != Confirmed || got.Item.Status != domain.Incomplete {
		t.Fatalf("got %s with %s, %v", got.Outcome, got.Item.Status, err)
	}
}

func TestReconcileReportsAChangeThatNeverHappened(t *testing.T) {
	f := newFixture(t)
	item := f.add("Study", "1h").Item
	state := f.read(Selection{})
	started := item.Clone()
	_ = started.Apply(domain.Start, f.now, state.Day)
	pending := Result{Outcome: Unconfirmed, At: f.now, Day: state.Day, Intended: started, Reason: domain.ReasonUser, BeforeSeq: state.Revisions[item.ID].Seq}
	got, err := f.client.Reconcile(ctx, pending)
	if err != nil || got.Outcome != Refused {
		t.Fatalf("got %s, %v", got.Outcome, err)
	}
	pending.Intended.ID = "it_never_saved"
	if got, err := f.client.Reconcile(ctx, pending); err != nil || got.Outcome != Refused {
		t.Fatalf("an add that never landed: %s, %v", got.Outcome, err)
	}
}

func TestReconcileAdmitsWhenItCannotTell(t *testing.T) {
	f := newFixture(t)
	item := f.add("Study", "1h").Item
	state := f.read(Selection{})
	started := item.Clone()
	_ = started.Apply(domain.Start, f.now, state.Day)
	pending := Result{Outcome: Unconfirmed, At: f.now.Add(-time.Minute), Day: state.Day, Intended: started, Reason: domain.ReasonUser, BeforeSeq: state.Revisions[item.ID].Seq}
	f.must(Skip, item.ID) // something else happened after the unknown change
	if _, err := f.client.Reconcile(ctx, pending); !errors.Is(err, ErrStillUnknown) {
		t.Fatal("an unprovable change must stay unknown", err)
	}
}
