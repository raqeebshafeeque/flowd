package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
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
		"a newer version":       {"CREATE TABLE days(x)", "PRAGMA user_version = 2"},
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
