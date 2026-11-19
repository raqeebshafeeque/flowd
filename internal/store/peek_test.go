package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPeekCreatesNothingWhenThereIsNoDatabase(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "flowd")
	g, err := Peek(ctx, filepath.Join(dir, "flowd.sqlite"), "2026-11-17")
	if err != nil || g.Running || g.Day.ID != "" {
		t.Fatalf("got %+v, %v", g, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("peeking must not create the folder")
	}
}

// A status line asks every second or two, so it must never wait for the
// lock or take it.
func TestPeekReadsWhileAnotherFlowdHoldsTheLock(t *testing.T) {
	s := openTest(t)
	day, _ := createTestItem(t, s)
	g, err := Peek(ctx, s.Path(), "2026-11-17")
	if err != nil || g.Day.ID != day.ID || g.Running {
		t.Fatalf("got %+v, %v", g, err)
	}
	if g, err := Peek(ctx, s.Path(), "2026-11-18"); err != nil || g.Day.ID != "" {
		t.Fatalf("an unplanned date: %+v, %v", g, err)
	}
}

func TestPeekRefusesAnotherVersionAndChangesNothing(t *testing.T) {
	path := writeRaw(t, "CREATE TABLE days(x)", "PRAGMA user_version = 4")
	before, _ := os.ReadFile(path)
	if _, err := Peek(ctx, path, "2026-11-17"); !errors.Is(err, ErrVersion) {
		t.Fatalf("got %v", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("the file changed")
	}
}

// Peeking leaves the quick check record valid, so the next real open still
// skips the full scan.
func TestPeekDoesNotForceAScan(t *testing.T) {
	s := openTest(t)
	createTestItem(t, s)
	path := s.Path()
	s.Close()
	if _, err := Peek(ctx, path, "2026-11-17"); err != nil {
		t.Fatal(err)
	}
	before := scans.Load()
	again, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	again.Close()
	if scans.Load() != before {
		t.Fatal("a peek made the next open scan the whole file")
	}
}
