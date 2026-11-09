package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreOwnership(t *testing.T) {
	first := openTestStore(t)
	path := first.Path()
	alias := filepath.Join(t.TempDir(), "alias.sqlite")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{path, alias} {
		second, err := Open(context.Background(), candidate)
		if second != nil {
			_ = second.Close()
		}
		if err == nil {
			t.Errorf("another open should not acquire %q", candidate)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Errorf("repeat close should succeed: %v", err)
	}
}

func TestFailedOpenReleasesOwnership(t *testing.T) {
	first := openTestStore(t)
	path := first.Path()
	if _, err := first.db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(context.Background(), path); opened != nil || !errors.Is(err, ErrVersion) {
		if opened != nil {
			_ = opened.Close()
		}
		t.Fatalf("want version refusal, got %v", err)
	}
	raw := openUninitializedStore(t, path)
	if _, err := raw.db.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("failed open leaked ownership: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHardLinkedDatabaseRefused(t *testing.T) {
	first := openTestStore(t)
	path := first.Path()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "hard-link.sqlite")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(context.Background(), alias)
	if opened != nil {
		_ = opened.Close()
	}
	if !errors.Is(err, ErrInvalidPath) {
		t.Errorf("hard-linked databases must be refused explicitly, got %v", err)
	}
}

func TestUnsafeCoordinationPath(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "flowd.sqlite")
	other := filepath.Join(directory, "other")
	if err := os.WriteFile(other, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, path+".lock"); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(context.Background(), path)
	if opened != nil {
		_ = opened.Close()
	}
	if err == nil {
		t.Error("coordination symlinks must be refused")
	}
	info, err := os.Stat(other)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Error("unexpected coordination target must not be chmodded")
	}
}
