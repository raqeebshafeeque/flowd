package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// damagedWithBackup makes a database with one item, backs it up, then
// overwrites the live file with garbage.
func damagedWithBackup(t *testing.T) (path string, item domain.Item) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "flowd.sqlite")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, item = createTestItem(t, s)
	if _, err := s.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := os.WriteFile(path, []byte(strings.Repeat("broken ", 2000)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, item
}

func TestRepairRestoresTheBackupAndKeepsTheOriginal(t *testing.T) {
	path, item := damagedWithBackup(t)
	recovery, err := Repair(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal("the repaired database does not open:", err)
	}
	defer s.Close()
	if _, err := s.Item(ctx, item.ID); err != nil {
		t.Fatal("the restored item is missing:", err)
	}
	kept, err := os.ReadFile(filepath.Join(recovery, filepath.Base(path)))
	if err != nil || !strings.HasPrefix(string(kept), "broken") {
		t.Fatal("the damaged original was not kept", err)
	}
	if info, _ := os.Stat(recovery); info.Mode().Perm() != 0o700 {
		t.Fatal("the recovery folder is not private")
	}
}

func TestRepairWithoutAUsableBackupChangesNothing(t *testing.T) {
	path, _ := damagedWithBackup(t)
	if err := os.WriteFile(path+".bak", []byte("not a backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if _, err := Repair(ctx, path); err == nil {
		t.Fatal("a broken backup must be refused")
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("the live file was changed")
	}
	os.Remove(path + ".bak")
	if _, err := Repair(ctx, path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a missing backup must be reported", err)
	}
}

// Reset moves everything aside and the next open starts empty. Nothing is
// deleted, so the old plan can still be put back by hand.
func TestResetKeepsTheOldFilesAndStartsEmpty(t *testing.T) {
	path := savedDatabase(t)
	recovery, err := Reset(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(recovery); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatal("the recovery folder is missing or not private", err)
	}
	kept, err := Open(ctx, filepath.Join(recovery, filepath.Base(path)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kept.DayByDate(ctx, "2026-11-17"); err != nil {
		t.Fatal("the old plan was not kept", err)
	}
	kept.Close()
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DayByDate(ctx, "2026-11-17"); !errors.Is(err, ErrNotFound) {
		t.Fatal("the new planner is not empty", err)
	}
	if _, err := Reset(ctx, filepath.Join(t.TempDir(), "none.sqlite")); !errors.Is(err, ErrNothingToReset) {
		t.Fatal("a missing database must be reported", err)
	}
}

func TestResetWaitsForNobody(t *testing.T) {
	s := openTest(t)
	if _, err := Reset(ctx, s.Path()); !errors.Is(err, ErrLocked) {
		t.Fatal("a database in use must not be reset", err)
	}
}
