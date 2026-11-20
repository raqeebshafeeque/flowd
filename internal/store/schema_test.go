package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// writeRaw makes a SQLite file by hand, the way another program might.
func writeRaw(t *testing.T, statements ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flowd.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestNewerOrForeignDatabasesAreRefused(t *testing.T) {
	cases := map[string][]string{
		"a newer version":       {"CREATE TABLE days(x)", "PRAGMA user_version = 5"},
		"someone else's tables": {"CREATE TABLE notes(text)"},
	}
	for name, statements := range cases {
		path := writeRaw(t, statements...)
		if err := chmodPrivate(path); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(context.Background(), path); !errors.Is(err, ErrVersion) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestWrongTableShapeIsDamage(t *testing.T) {
	path := writeRaw(t, "CREATE TABLE days(id)", "CREATE TABLE items(id)", "CREATE TABLE item_events(seq)", "PRAGMA user_version = 1")
	if err := chmodPrivate(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), path); !errors.Is(err, ErrCorrupt) {
		t.Fatal("tables with missing columns must be refused", err)
	}
}

func TestEventsCannotBeChangedOrDeleted(t *testing.T) {
	s := openTest(t)
	_, item := createTestItem(t, s)
	for _, statement := range []string{"UPDATE item_events SET reason='user'", "DELETE FROM item_events"} {
		if _, err := s.db.Exec(statement); err == nil {
			t.Errorf("%q was allowed", statement)
		}
	}
	if events, _ := s.Events(context.Background(), item.ID); len(events) != 1 {
		t.Fatal("the event is gone")
	}
}

// olderFile turns a closed database back into the given schema version, the
// way an older flowd left it, by dropping what later versions added.
func olderFile(t *testing.T, path string, v int) {
	t.Helper()
	steps := map[int][]string{
		3: {"ALTER TABLE items DROP COLUMN corrected_at"},
		2: {"DROP INDEX items_in_order", "ALTER TABLE items DROP COLUMN position"},
		1: {"DROP INDEX items_by_origin", "ALTER TABLE items DROP COLUMN carried_from"},
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for from := SchemaVersion - 1; from >= v; from-- {
		for _, statement := range steps[from] {
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(statement, err)
			}
		}
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", v)); err != nil {
		t.Fatal(err)
	}
}

// versionOne makes a file the way the first releases left it, with the history setup
// makes, or one item when setup is nil.
func versionOne(t *testing.T, setup func(*Store)) string {
	t.Helper()
	s := openTest(t)
	if setup == nil {
		createTestItem(t, s)
	} else {
		setup(s)
	}
	path := s.Path()
	s.Close()
	olderFile(t, path, 1)
	return path
}

func version(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Opening a version 1 file to write backs it up and upgrades it, and
// everything saved before is still there and still checks out.
func TestAnOlderFileIsBackedUpAndUpgraded(t *testing.T) {
	path := versionOne(t, nil)
	if err := Inspect(ctx, path); err != nil {
		t.Fatalf("doctor should accept a file it will upgrade: %v", err)
	}
	if version(t, path) != 1 {
		t.Fatal("inspecting must not upgrade")
	}
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if version(t, path) != SchemaVersion {
		t.Fatalf("still version %d", version(t, path))
	}
	if version(t, path+".bak") != 1 {
		t.Fatal("the backup should be the file as it was before the upgrade")
	}
	day, err := s.DayByDate(ctx, "2026-11-17")
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.Items(ctx, day.ID)
	if err != nil || len(items) != 1 || items[0].CarriedFrom != nil {
		t.Fatalf("items %+v, %v", items, err)
	}
	history, err := s.DayEvents(ctx, day.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.Measure(day, items[0], history[items[0].ID], clock("12:00:00")); err != nil {
		t.Fatalf("the old history should still check out: %v", err)
	}
}

// If the backup cannot be written, nothing is changed.
func TestAFailedUpgradeLeavesTheFileAsItWas(t *testing.T) {
	path := versionOne(t, nil)
	if err := os.Mkdir(path+".bak", 0o700); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if _, err := Open(ctx, path); err == nil {
		t.Fatal("the upgrade should have stopped")
	}
	if version(t, path) != 1 {
		t.Fatal("the file was changed")
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("the file's contents changed")
	}
}

// The lock free reads never upgrade. They see nothing until a real command
// has upgraded the file.
func TestPeekingNeverUpgrades(t *testing.T) {
	path := versionOne(t, nil)
	if _, err := Peek(ctx, path, "2026-11-17"); !errors.Is(err, ErrVersion) {
		t.Fatalf("got %v", err)
	}
	if version(t, path) != 1 {
		t.Fatal("peeking upgraded the file")
	}
}

// The upgrade numbers every day in the order it showed before: started
// items first, by their first start, then the rest as they were added.
func TestTheUpgradeKeepsEachDaysOrder(t *testing.T) {
	path := versionOne(t, func(s *Store) {
		day, _ := createTestItem(t, s)
		ids := plan(t, s, day, "08:10:00", "B", "C")
		c, _ := s.Item(ctx, ids[1])
		act(t, s, day, c, domain.Start, clock("09:00:00"))
	})
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	day, _ := s.DayByDate(ctx, "2026-11-17")
	if got := order(t, s, day); got != "C Study B" {
		t.Fatalf("got %s, want the started item first and the rest as added", got)
	}
}

// A version 2 file, which has carried_from but no position, goes on to the
// current version with everything in it, a carried item included.
func TestAVersionTwoFileIsUpgraded(t *testing.T) {
	s := openTest(t)
	day, first := createTestItem(t, s)
	copied, _ := domain.NewItem(day.ID, "Carried", 30, clock("08:30:00"))
	from := first.ID
	copied.CarriedFrom = &from
	if err := s.Create(ctx, nil, Change{Item: copied, Event: domain.Record(nil, copied, copied.CreatedAt, copied.CreatedAt, domain.ReasonUser)}); err != nil {
		t.Fatal(err)
	}
	path := s.Path()
	s.Close()
	olderFile(t, path, 2)
	if err := Inspect(ctx, path); err != nil {
		t.Fatalf("doctor should accept a version 2 file: %v", err)
	}
	again, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if version(t, path) != SchemaVersion || version(t, path+".bak") != 2 {
		t.Fatalf("version %d, backup %d", version(t, path), version(t, path+".bak"))
	}
	items, err := again.Items(ctx, day.ID)
	if err != nil || len(items) != 2 || items[1].CarriedFrom == nil || *items[1].CarriedFrom != first.ID {
		t.Fatalf("items %+v, %v", items, err)
	}
	if got := order(t, again, day); got != "Study Carried" {
		t.Fatalf("got %s", got)
	}
}

// A version 3 file, with its order but no corrections, gains corrected_at
// and keeps its order.
func TestAVersionThreeFileIsUpgraded(t *testing.T) {
	s := openTest(t)
	day, _ := createTestItem(t, s)
	plan(t, s, day, "08:10:00", "B")
	path := s.Path()
	s.Close()
	olderFile(t, path, 3)
	again, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if version(t, path) != SchemaVersion || version(t, path+".bak") != 3 {
		t.Fatalf("version %d, backup %d", version(t, path), version(t, path+".bak"))
	}
	if got := order(t, again, day); got != "Study B" {
		t.Fatalf("got %s", got)
	}
}

// Items added in the same second keep a fixed order, by ID.
func TestTheOldOrderBreaksTiesByID(t *testing.T) {
	a := domain.Item{ID: "it_a", CreatedAt: clock("08:00:00")}
	b, later := a, a
	b.ID, later.ID, later.CreatedAt = "it_b", "it_0", clock("08:00:01")
	items := []domain.Item{later, b, a}
	slices.SortStableFunc(items, compareAsBefore)
	if items[0].ID != "it_a" || items[1].ID != "it_b" || items[2].ID != "it_0" {
		t.Fatalf("got %v %v %v", items[0].ID, items[1].ID, items[2].ID)
	}
}
