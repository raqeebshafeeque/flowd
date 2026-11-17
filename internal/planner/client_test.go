package planner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

var ctx = context.Background()

// fixture is a planner on a fresh database with a clock the test controls.
type fixture struct {
	t      *testing.T
	client Client
	now    time.Time
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
	f.client = Client{Path: filepath.Join(t.TempDir(), "flowd.sqlite"), Now: func() time.Time { return f.now }, Zone: time.UTC}
	return f
}

func (f *fixture) at(clock string) {
	t, err := time.Parse("2006-01-02 15:04:05", "2026-11-17 "+clock)
	if err != nil {
		f.t.Fatal(err)
	}
	f.now = t
}

func (f *fixture) read(selection Selection) State {
	f.t.Helper()
	state, err := f.client.Read(ctx, selection)
	if err != nil {
		f.t.Fatal(err)
	}
	return state
}

func text(s string) *string { return &s }

func (f *fixture) add(title, duration string) Result {
	f.t.Helper()
	r, err := f.client.Do(ctx, Intent{Operation: Add, Title: text(title), Duration: text(duration)})
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}

// change performs an operation on an item using its latest revision.
func (f *fixture) change(op Operation, id domain.ID, extra ...func(*Intent)) (Result, error) {
	f.t.Helper()
	state := f.read(Selection{})
	intent := Intent{Operation: op, ItemID: id, Revision: state.Revisions[id]}
	for _, apply := range extra {
		apply(&intent)
	}
	return f.client.Do(ctx, intent)
}

func (f *fixture) must(op Operation, id domain.ID) Result {
	f.t.Helper()
	r, err := f.change(op, id)
	if err != nil {
		f.t.Fatalf("%s: %v", op, err)
	}
	return r
}

func TestReadingAnEmptyDayCreatesNothing(t *testing.T) {
	f := newFixture(t)
	state := f.read(Selection{Date: "2026-11-20"})
	if state.Day.ID != "" || len(state.View.Items) != 0 || state.View.Date != "2026-11-20" {
		t.Fatalf("unexpected state: %+v", state)
	}
	if state = f.read(Selection{}); state.Day.Date != "2026-11-17" || state.Day.Timezone != "UTC" {
		t.Fatalf("today was not chosen from the clock and zone: %+v", state.Day)
	}
}

func TestADayKeepsItsTimezone(t *testing.T) {
	f := newFixture(t)
	if _, err := f.client.Do(ctx, Intent{Operation: Add, Selection: Selection{Timezone: "Asia/Kolkata"}, Title: text("Study"), Duration: text("1h")}); err != nil {
		t.Fatal(err)
	}
	if state := f.read(Selection{}); state.Day.Timezone != "Asia/Kolkata" {
		t.Fatalf("got %q", state.Day.Timezone)
	}
	if _, err := f.client.Read(ctx, Selection{Timezone: "UTC"}); !errors.Is(err, ErrInput) {
		t.Fatal("asking for another timezone on a saved day must be refused", err)
	}
	if _, err := f.client.Read(ctx, Selection{Date: "2026-02-30"}); !errors.Is(err, ErrInput) {
		t.Fatal("an impossible date must be refused", err)
	}
}

func TestStaleChangesAreRefused(t *testing.T) {
	f := newFixture(t)
	item := f.add("Study", "1h").Item
	old := f.read(Selection{}).Revisions[item.ID]
	f.must(Start, item.ID)
	if _, err := f.client.Do(ctx, Intent{Operation: Skip, ItemID: item.ID, Revision: old}); !errors.Is(err, ErrStale) {
		t.Fatal("a change based on an old read must be refused", err)
	}
}

func TestSystemZoneUsesANamedZone(t *testing.T) {
	dir := t.TempDir()
	zoneFile := filepath.Join(dir, "timezone")
	if err := os.WriteFile(zoneFile, []byte("Asia/Kolkata\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		env, file, want string
	}{
		{"Europe/Paris", zoneFile, "Europe/Paris"},
		{":America/New_York", "", "America/New_York"},
		{"", zoneFile, "Asia/Kolkata"},
		{"Local", "", "UTC"},
		{"Not/AZone", "", "UTC"},
	}
	for _, c := range cases {
		if got := detectZone(c.env, filepath.Join(dir, "missing"), c.file).String(); got != c.want {
			t.Errorf("env %q file %q: got %s, want %s", c.env, c.file, got, c.want)
		}
	}
}
