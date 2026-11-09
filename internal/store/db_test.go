package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestOpen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "flowd.sqlite")

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if store == nil || store.db == nil {
		t.Fatal("want a store with a database handle")
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("could not close database: %v", err)
		}
	})

	var tableCount int
	if err := store.db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_schema
		WHERE type = 'table' AND name IN ('days', 'blocks', 'block_events')
	`).Scan(&tableCount); err != nil {
		t.Fatalf("could not inspect initialized tables: %v", err)
	}
	if tableCount != 3 {
		t.Errorf("got %d planner tables after opening, want 3", tableCount)
	}

	file, err := os.Stat(path)
	if err != nil {
		t.Fatalf("database missing at requested path: %v", err)
	}
	if !file.Mode().IsRegular() {
		t.Error("want a regular database file at the requested path")
	}

	if _, err := store.db.ExecContext(ctx, "CREATE TABLE notes (title TEXT NOT NULL)"); err != nil {
		t.Fatalf("could not create test table: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "INSERT INTO notes (title) VALUES (?)", "study Go"); err != nil {
		t.Fatalf("could not save test note: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("could not close database before reopening: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("could not reopen database: %v", err)
	}
	if reopened == nil || reopened.db == nil {
		t.Fatal("want a usable store after reopening")
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("could not close reopened database: %v", err)
		}
	})

	var title string
	if err := reopened.db.QueryRowContext(ctx, "SELECT title FROM notes").Scan(&title); err != nil {
		t.Fatalf("could not read saved note: %v", err)
	}
	if title != "study Go" {
		t.Errorf("got title %q, want %q", title, "study Go")
	}
}

func TestOpenErrors(t *testing.T) {
	directory := t.TempDir()
	parentFile := filepath.Join(directory, "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		path    string
		wantErr error
	}{
		{
			name:    "cancelled context preserves its cause",
			ctx:     cancelledContext,
			path:    filepath.Join(directory, "cancelled.sqlite"),
			wantErr: context.Canceled,
		},
		{
			name:    "directory cannot be a database file",
			ctx:     context.Background(),
			path:    directory,
			wantErr: ErrInvalidPath,
		},
		{
			name: "a regular file cannot be a parent directory",
			ctx:  context.Background(),
			path: filepath.Join(parentFile, "flowd.sqlite"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := Open(tt.ctx, tt.path)
			if store != nil {
				t.Error("got a store after failed open, want nil")
				if closeErr := store.Close(); closeErr != nil {
					t.Errorf("could not clean up unexpected store: %v", closeErr)
				}
			}
			if err == nil {
				t.Fatal("want an error when opening fails")
			}
			if !strings.Contains(err.Error(), tt.path) {
				t.Errorf("error is missing database path %q: %v", tt.path, err)
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}

			var pathError *os.PathError
			if !errors.As(err, &pathError) {
				t.Errorf("want original filesystem cause, got %v", err)
			}
		})
	}
}

func TestOpenSchemaErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   string
		wantErr error
	}{
		{
			name:    "unsupported schema preserves its cause",
			setup:   "PRAGMA user_version = 2",
			wantErr: ErrVersion,
		},
		{
			name:  "conflicting schema preserves its cause",
			setup: "CREATE TABLE blocks (title TEXT)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "flowd.sqlite")
			existing := openUninitializedStore(t, path)
			if _, err := existing.db.ExecContext(context.Background(), tt.setup); err != nil {
				t.Fatalf("could not prepare test schema: %v", err)
			}
			if err := existing.Close(); err != nil {
				t.Fatalf("could not close test database: %v", err)
			}

			store, err := Open(context.Background(), path)
			if store != nil {
				t.Error("got a store after failed schema setup, want nil")
				if closeErr := store.Close(); closeErr != nil {
					t.Errorf("could not clean up unexpected store: %v", closeErr)
				}
			}
			if err == nil {
				t.Fatal("want an error when schema setup fails")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error is missing database path %q: %v", path, err)
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			var sqliteError *sqlite.Error
			if !errors.As(err, &sqliteError) {
				t.Errorf("want the original SQLite setup error preserved, got %v", err)
			}
		})
	}
}

func TestOpenPaths(t *testing.T) {
	tests := []struct {
		name     string
		filename string
	}{
		{name: "spaces stay in the filename", filename: "flowd notes.sqlite"},
		{name: "query characters stay in the filename", filename: "flowd?mode=ro#notes.sqlite"},
		{name: "percent signs stay in the filename", filename: "flowd%notes.sqlite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tt.filename)
			store, err := Open(context.Background(), path)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			t.Cleanup(func() {
				if err := store.Close(); err != nil {
					t.Errorf("could not close database: %v", err)
				}
			})
			if _, err := os.Stat(path); err != nil {
				t.Errorf("database missing at requested path: %v", err)
			}
		})
	}
}

func TestConnectionSettings(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "flowd.sqlite"))
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("could not close database: %v", err)
		}
	})

	_, err = store.db.ExecContext(ctx, `
		CREATE TABLE parents (id TEXT NOT NULL PRIMARY KEY);
		CREATE TABLE children (parent_id TEXT NOT NULL REFERENCES parents(id));
		INSERT INTO parents (id) VALUES ('existing');
	`)
	if err != nil {
		t.Fatalf("could not prepare test tables: %v", err)
	}

	// Discard returned connections so each subtest opens a fresh one.
	store.db.SetMaxIdleConns(0)
	for _, name := range []string{"first connection", "replacement connection"} {
		t.Run(name, func(t *testing.T) {
			connection, err := store.db.Conn(ctx)
			if err != nil {
				t.Fatalf("could not obtain connection: %v", err)
			}
			t.Cleanup(func() {
				if err := connection.Close(); err != nil {
					t.Errorf("could not release connection: %v", err)
				}
			})

			settings := []struct {
				name  string
				query string
				want  string
			}{
				{name: "foreign keys", query: "PRAGMA foreign_keys", want: "1"},
				{name: "journal mode", query: "PRAGMA journal_mode", want: "wal"},
				{name: "synchronous mode", query: "PRAGMA synchronous", want: "2"},
			}
			for _, setting := range settings {
				t.Run(setting.name, func(t *testing.T) {
					var got string
					if err := connection.QueryRowContext(ctx, setting.query).Scan(&got); err != nil {
						t.Fatalf("could not read setting: %v", err)
					}
					if got != setting.want {
						t.Errorf("got setting %q, want %q", got, setting.want)
					}
				})
			}

			if _, err := connection.ExecContext(ctx, "INSERT INTO children (parent_id) VALUES (?)", "existing"); err != nil {
				t.Fatalf("existing parent should be accepted: %v", err)
			}
			_, err = connection.ExecContext(ctx, "INSERT INTO children (parent_id) VALUES (?)", "missing")
			var sqliteError *sqlite.Error
			if !errors.As(err, &sqliteError) {
				t.Fatalf("want a SQLite foreign-key error for missing parent, got %v", err)
			}
			if code := sqliteError.Code(); code != sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY {
				t.Errorf("got SQLite code %d, want %d", code, sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY)
			}
		})
	}
}

func TestClose(t *testing.T) {
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "flowd.sqlite"))
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if store == nil || store.db == nil {
		t.Fatal("want a store with a database handle")
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("could not clean up database: %v", err)
		}
	})

	if err := store.Close(); err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if err := store.db.PingContext(context.Background()); err == nil {
		t.Error("database still accepts connections after close")
	}
	if err := store.Close(); err != nil {
		t.Errorf("closing an already closed store should succeed: %v", err)
	}
}
