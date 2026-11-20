package planner

import (
	"testing"
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
