package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func TestBackupIsAPrivateCompleteCopy(t *testing.T) {
	s := openTest(t)
	day, item := createTestItem(t, s)
	act(t, s, day, item, domain.Start, clock("09:00:00"))
	path, err := s.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if path != s.Path()+".bak" {
		t.Fatalf("backup written to %q", path)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode %v, %v", info.Mode().Perm(), err)
	}
	copy, err := Open(ctx, path)
	if err != nil {
		t.Fatal("the backup does not open:", err)
	}
	defer copy.Close()
	saved, err := copy.Item(ctx, item.ID)
	if err != nil || saved.Status != domain.InProgress {
		t.Fatalf("the backup is missing the latest change: %+v %v", saved, err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".flowd-backup-*")); len(leftovers) != 0 {
		t.Fatal("temporary backup files were left behind", leftovers)
	}
}

func TestBackupReplacesTheOldOne(t *testing.T) {
	s := openTest(t)
	day, item := createTestItem(t, s)
	if _, err := s.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	act(t, s, day, item, domain.Skip, clock("09:00:00"))
	path, err := s.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	copy, _ := Open(ctx, path)
	defer copy.Close()
	if saved, _ := copy.Item(ctx, item.ID); saved.Status != domain.Skipped {
		t.Fatal("the second backup did not replace the first")
	}
}

func TestInspect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flowd.sqlite")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Inspect(ctx, path); !errors.Is(err, ErrLocked) {
		t.Fatal("inspect must notice another owner", err)
	}
	s.Close()
	if err := Inspect(ctx, path); err != nil {
		t.Fatal("a healthy database must pass:", err)
	}
	if err := Inspect(ctx, filepath.Join(t.TempDir(), "missing.sqlite")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a missing database must be reported", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Inspect(ctx, path); !errors.Is(err, ErrPermissions) {
		t.Fatal("a shared database must be reported", err)
	}
	garbage := filepath.Join(t.TempDir(), "flowd.sqlite")
	if err := os.WriteFile(garbage, make([]byte, 8192), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Inspect(ctx, garbage); !errors.Is(err, ErrCorrupt) {
		t.Fatal("damage must be reported", err)
	}
}
