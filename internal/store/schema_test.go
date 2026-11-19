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
		"a newer version":       {"CREATE TABLE days(x)", "PRAGMA user_version = 3"},
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
	s := openTest(t)
	createTestItem(t, s)
	path := s.Path()
	s.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{"DROP INDEX items_by_origin", "ALTER TABLE items DROP COLUMN carried_from", "PRAGMA user_version = 1"} {
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
