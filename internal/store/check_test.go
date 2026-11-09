package store

import (
	"bytes"
	"context"
	"errors"

	"modernc.org/sqlite"
	"os"
	"path/filepath"
	"testing"
)

func TestOpeningIncompleteSchema(t *testing.T) {
	for _, test := range []struct{ name, damage string }{{"missing table", "DROP TABLE block_events"}, {"missing column", "ALTER TABLE days DROP COLUMN timezone"}} {
		t.Run(test.name, func(t *testing.T) {
			store := openTestStore(t)
			path := store.Path()
			if _, err := store.db.Exec(test.damage); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			opened, err := Open(context.Background(), path)
			if opened != nil {
				_ = opened.Close()
			}
			if !errors.Is(err, ErrCorrupt) {
				t.Errorf("want corruption refusal for incomplete schema, got %v", err)
			}
		})
	}
}

func TestOpeningGarbagePreservesBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flowd.sqlite")
	original := []byte("not a database")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(context.Background(), path)
	if opened != nil {
		_ = opened.Close()
	}
	var cause *sqlite.Error
	if !errors.Is(err, ErrCorrupt) || !errors.As(err, &cause) {
		t.Fatalf("want corruption sentinel and driver cause, got %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, original) {
		t.Error("failed open must preserve original bytes")
	}
}
