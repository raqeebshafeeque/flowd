package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"modernc.org/sqlite"
)

func TestInitializeSchema(t *testing.T) {
	store := openUninitializedStore(t, filepath.Join(t.TempDir(), "flowd.sqlite"))
	ctx := context.Background()
	if err := store.initializeSchema(ctx); err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	var version int
	if err := store.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("could not read schema version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("got schema version %d, want %d", version, schemaVersion)
	}

	tests := []struct {
		name  string
		kind  string
		table string
	}{
		{name: "days", kind: "table", table: "days"},
		{name: "blocks", kind: "table", table: "blocks"},
		{name: "block_events", kind: "table", table: "block_events"},
		{name: "blocks_by_day_and_start", kind: "index", table: "blocks"},
		{name: "events_by_block_and_sequence", kind: "index", table: "block_events"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var table string
			err := store.db.QueryRowContext(ctx,
				"SELECT tbl_name FROM sqlite_schema WHERE name = ? AND type = ?",
				tt.name, tt.kind,
			).Scan(&table)
			if err != nil {
				t.Fatalf("could not find schema object: %v", err)
			}
			if table != tt.table {
				t.Errorf("got owning table %q, want %q", table, tt.table)
			}
		})
	}
}

func TestInitializeSchemaPreservesData(t *testing.T) {
	store := openUninitializedStore(t, filepath.Join(t.TempDir(), "flowd.sqlite"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := store.initializeSchema(ctx); err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO days (id, date, timezone, created_at_utc)
		VALUES ('day', '2026-10-14', 'Asia/Kolkata', '2026-10-14T00:00:00Z');

		INSERT INTO blocks
			(id, day_id, title, status, planned_start_utc, planned_end_utc, created_at_utc)
		VALUES
			('study', 'day', 'Study Go', 'todo', '2026-10-14T03:30:00Z',
			 '2026-10-14T04:30:00Z', '2026-10-14T00:00:00Z');

		INSERT INTO block_events (block_id, from_status, to_status, occurred_at_utc, reason)
		VALUES ('study', 'todo', 'todo', '2026-10-14T00:00:00Z', 'user');
	`)
	if err != nil {
		t.Fatalf("could not save test data: %v", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		if err := store.initializeSchema(ctx); err != nil {
			t.Fatalf("existing schema should be accepted: %v", err)
		}
		if inUse := store.db.Stats().InUse; inUse != 0 {
			t.Fatalf("got %d connections still in use, want 0", inUse)
		}
	}

	var title, date, reason string
	err = store.db.QueryRowContext(ctx, `
		SELECT blocks.title, days.date, block_events.reason
		FROM blocks
		JOIN days ON days.id = blocks.day_id
		JOIN block_events ON block_events.block_id = blocks.id
		WHERE blocks.id = 'study'
	`).Scan(&title, &date, &reason)
	if err != nil {
		t.Fatalf("could not read existing day, block and event: %v", err)
	}
	if title != "Study Go" || date != "2026-10-14" || reason != "user" {
		t.Errorf("got title %q date %q reason %q, want Study Go, 2026-10-14 and user", title, date, reason)
	}
}

func TestInitializeSchemaUnsupportedVersion(t *testing.T) {
	tests := []struct {
		name    string
		version int
	}{
		{name: "newer schema is refused", version: schemaVersion + 1},
		{name: "negative schema version is refused", version: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := openUninitializedStore(t, filepath.Join(t.TempDir(), "flowd.sqlite"))
			ctx := context.Background()
			_, err := store.db.ExecContext(ctx, `
				CREATE TABLE notes (title TEXT NOT NULL);
				INSERT INTO notes (title) VALUES ('keep this note');
			`)
			if err != nil {
				t.Fatalf("could not save test note: %v", err)
			}
			if _, err := store.db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", tt.version)); err != nil {
				t.Fatalf("could not set test schema version: %v", err)
			}

			if err := store.initializeSchema(ctx); !errors.Is(err, ErrVersion) {
				t.Errorf("got error %v, want %v", err, ErrVersion)
			}
			if inUse := store.db.Stats().InUse; inUse != 0 {
				t.Fatalf("got %d connections still in use, want 0", inUse)
			}

			var version int
			if err := store.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
				t.Fatalf("could not read schema version: %v", err)
			}
			if version != tt.version {
				t.Errorf("got schema version %d, want unchanged %d", version, tt.version)
			}
			var title string
			if err := store.db.QueryRowContext(ctx, "SELECT title FROM notes").Scan(&title); err != nil {
				t.Fatalf("could not read existing note: %v", err)
			}
			if title != "keep this note" {
				t.Errorf("got title %q, want %q", title, "keep this note")
			}
			var count int
			if err := store.db.QueryRowContext(ctx,
				"SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'days'",
			).Scan(&count); err != nil {
				t.Fatalf("could not inspect tables: %v", err)
			}
			if count != 0 {
				t.Error("unsupported database should not gain planner tables")
			}
		})
	}
}

func TestInitializeSchemaRollback(t *testing.T) {
	store := openUninitializedStore(t, filepath.Join(t.TempDir(), "flowd.sqlite"))
	ctx := context.Background()

	// The schema creates days first, then fails because blocks already exists.
	_, err := store.db.ExecContext(ctx, `
		CREATE TABLE blocks (title TEXT NOT NULL);
		INSERT INTO blocks (title) VALUES ('keep this block');
	`)
	if err != nil {
		t.Fatalf("could not prepare conflicting table: %v", err)
	}

	err = store.initializeSchema(ctx)
	var sqliteError *sqlite.Error
	if !errors.As(err, &sqliteError) {
		t.Fatalf("want the original SQLite setup error preserved, got %v", err)
	}
	if inUse := store.db.Stats().InUse; inUse != 0 {
		t.Fatalf("got %d connections still in use, want 0", inUse)
	}

	var version, partialTables int
	if err := store.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("could not read schema version: %v", err)
	}
	if version != 0 {
		t.Errorf("got schema version %d after failure, want 0", version)
	}
	if err := store.db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_schema
		WHERE type = 'table' AND name IN ('days', 'block_events')
	`).Scan(&partialTables); err != nil {
		t.Fatalf("could not inspect partial schema: %v", err)
	}
	if partialTables != 0 {
		t.Errorf("got %d partial tables after failure, want 0", partialTables)
	}

	var title string
	if err := store.db.QueryRowContext(ctx, "SELECT title FROM blocks").Scan(&title); err != nil {
		t.Fatalf("could not read existing block: %v", err)
	}
	if title != "keep this block" {
		t.Errorf("got title %q, want %q", title, "keep this block")
	}

	if _, err := store.db.ExecContext(ctx, "DROP TABLE blocks"); err != nil {
		t.Fatalf("could not remove test conflict: %v", err)
	}
	if err := store.initializeSchema(ctx); err != nil {
		t.Fatalf("setup should succeed after removing the conflict: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("could not read schema version after retry: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("got schema version %d after retry, want %d", version, schemaVersion)
	}
}

func TestInitializeSchemaCancellation(t *testing.T) {
	store := openUninitializedStore(t, filepath.Join(t.TempDir(), "flowd.sqlite"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.initializeSchema(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("got error %v, want %v", err, context.Canceled)
	}
	if err := store.initializeSchema(context.Background()); err != nil {
		t.Fatalf("setup should succeed with a fresh context: %v", err)
	}
}

func openUninitializedStore(t *testing.T, path string) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", generateDatabaseURL(path))
	if err != nil {
		t.Fatalf("could not prepare database: %v", err)
	}
	// A leaked transaction would otherwise be hidden by another pool connection.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("could not close test database: %v", err)
		}
	})
	return &Store{db: db}
}
