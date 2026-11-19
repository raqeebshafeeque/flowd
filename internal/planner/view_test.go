package planner

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func TestViewOrdersMeasuresAndTotals(t *testing.T) {
	f := newFixture(t)
	a := f.add("A", "1h").Item
	b := f.add("B", "30m").Item
	f.must(Start, b.ID)
	f.at("09:20:00")
	f.must(Finish, b.ID)
	f.must(Start, a.ID)
	f.at("09:50:00")
	v := f.read(Selection{}).View
	if v.Items[0].ID != b.ID || v.Items[1].ID != a.ID {
		t.Fatal("the item started first must be listed first")
	}
	if v.Running == nil || v.Running.ID != a.ID || v.Running.WorkNS != int64(30*time.Minute) {
		t.Fatalf("running: %+v", v.Running)
	}
	if v.DurationMinutes.Int64() != 90 || v.WorkNS != int64(50*time.Minute) || v.Counts[domain.Done] != 1 {
		t.Fatalf("totals: %v %v %v", v.DurationMinutes, time.Duration(v.WorkNS), v.Counts)
	}
}

func TestViewJSONShape(t *testing.T) {
	f := newFixture(t)
	f.add("Study", "1h")
	raw, err := json.Marshal(f.read(Selection{}).View)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"schema_version":2`, `"date":"2026-11-17"`, `"items":[`, `"duration_minutes":60`, `"work_ns":0`, `"paused_ns":null`, `"running":null`, `"status_counts":{`} {
		if !strings.Contains(string(raw), field) {
			t.Errorf("missing %s in %s", field, raw)
		}
	}
}
