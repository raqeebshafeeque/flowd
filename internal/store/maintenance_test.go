package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInspect(t *testing.T) {
	ctx := context.Background()
	t.Run("missing parent stays missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing", "day.sqlite")
		if err := Inspect(ctx, path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("want missing cause, got %v", err)
		}
		if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
			t.Error("inspection created a directory")
		}
	})
	t.Run("active repair ownership takes precedence", func(t *testing.T) {
		db := blockTestStore(t)
		if err := os.WriteFile(db.Path()+".repair", []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Inspect(ctx, db.Path()); !errors.Is(err, ErrLocked) {
			t.Errorf("inspection must respect active repair ownership: %v", err)
		}
	})
	t.Run("healthy loose and held", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "day.sqlite")
		db, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		if err := Inspect(ctx, path); !errors.Is(err, ErrLocked) {
			t.Errorf("want lock error, got %v", err)
		}
		db.Close()
		if err := Inspect(ctx, path); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Inspect(ctx, path); !errors.Is(err, ErrPermissions) {
			t.Errorf("want private permission error, got %v", err)
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o644 {
			t.Error("inspection changed permissions")
		}
	})
	t.Run("no initialization", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.sqlite")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Inspect(ctx, path); !errors.Is(err, ErrVersion) {
			t.Errorf("want unsupported empty schema, got %v", err)
		}
		info, _ := os.Stat(path)
		if info.Size() != 0 {
			t.Error("inspection initialized an empty file")
		}
	})
	t.Run("foreign key damage", func(t *testing.T) {
		db := blockTestStore(t)
		block := exampleBlock(t, "bl_orphan", "09:00", "10:00")
		saveBlockFixture(t, db, block)
		if _, err := db.db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.db.Exec("DELETE FROM days"); err != nil {
			t.Fatal(err)
		}
		path := db.Path()
		db.Close()
		if err := Inspect(ctx, path); !errors.Is(err, ErrCorrupt) {
			t.Errorf("want foreign key corruption, got %v", err)
		}
	})
}
