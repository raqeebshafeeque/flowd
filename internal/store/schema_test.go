package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
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

// versionOne makes a file the way flowd 0.9 left it: history, but no
// carried_from column and user_version 1.
func versionOne(t *testing.T) string {
	t.Helper()
	return versionOneWith(t, func(s *Store) { createTestItem(t, s) })
}

// versionOneWith is versionOne with the history setup makes.
func versionOneWith(t *testing.T, setup func(*Store)) string {
	t.Helper()
	s := openTest(t)
	setup(s)
	path := s.Path()
	s.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{"DROP INDEX items_by_origin", "DROP INDEX items_in_order", "ALTER TABLE items DROP COLUMN carried_from", "ALTER TABLE items DROP COLUMN position", "ALTER TABLE items DROP COLUMN corrected_at", "PRAGMA user_version = 1"} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(statement, err)
		}
	}
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
	path := versionOne(t)
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
	if version(t, path) != SchemaVersion {
		t.Fatalf("a version 1 file should go all the way to the current version, got %d", version(t, path))
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
	path := versionOne(t)
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
	path := versionOne(t)
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
	path := versionOneWith(t, func(s *Store) {
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
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"DROP INDEX items_in_order", "ALTER TABLE items DROP COLUMN position", "ALTER TABLE items DROP COLUMN corrected_at", "PRAGMA user_version = 2"} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(statement, err)
		}
	}
	db.Close()
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
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"ALTER TABLE items DROP COLUMN corrected_at", "PRAGMA user_version = 3"} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(statement, err)
		}
	}
	db.Close()
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
