package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "flowd.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenCreatesAPrivateDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "flowd.sqlite")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("database mode %v, %v", info.Mode().Perm(), err)
	}
	dir, _ := os.Stat(filepath.Dir(path))
	if dir.Mode().Perm() != 0o700 {
		t.Fatalf("new folder mode %v", dir.Mode().Perm())
	}
	again, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal("reopening failed:", err)
	}
	again.Close()
}

func TestSecondOpenIsRefusedWhileTheFirstIsOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flowd.sqlite")
	first, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), path); !errors.Is(err, ErrLocked) {
		t.Fatal("a second owner must be refused", err)
	}
	first.Close()
	second, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal("the lock was not released:", err)
	}
	second.Close()
}

func TestDamagedFilesAreRefusedAndKept(t *testing.T) {
	cases := map[string][]byte{
		"empty file":     {},
		"not a database": []byte(strings.Repeat("this is not sqlite ", 300)),
	}
	for name, content := range cases {
		path := filepath.Join(t.TempDir(), "flowd.sqlite")
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(context.Background(), path); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: got %v", name, err)
		}
		if after, _ := os.ReadFile(path); string(after) != string(content) {
			t.Errorf("%s: the file was changed", name)
		}
	}
}

func TestUnsafePathsAreRefused(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.sqlite")
	s, err := Open(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	hard := filepath.Join(dir, "hard.sqlite")
	if err := os.Link(target, hard); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), hard); !errors.Is(err, ErrInvalidPath) {
		t.Error("a hard-linked database must be refused", err)
	}
	if _, err := Open(context.Background(), dir); !errors.Is(err, ErrInvalidPath) {
		t.Error("a directory must be refused", err)
	}
	if _, err := Open(context.Background(), ""); !errors.Is(err, ErrInvalidPath) {
		t.Error("an empty path must be refused", err)
	}
}

func TestSharedFilesAreRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flowd.sqlite")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), path); !errors.Is(err, ErrPermissions) {
		t.Fatal("a database others can read must be refused", err)
	}
}

func TestSymlinkToADatabaseIsFollowed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.sqlite")
	created, err := Open(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	created.Close()
	link := filepath.Join(dir, "link.sqlite")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if want, _ := filepath.EvalSymlinks(target); s.Path() != want {
		t.Fatalf("got path %q, want the link target %q", s.Path(), want)
	}
	dangling := filepath.Join(dir, "dangling.sqlite")
	if err := os.Symlink(filepath.Join(dir, "missing.sqlite"), dangling); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), dangling); !errors.Is(err, ErrInvalidPath) {
		t.Fatal("a link to nothing must be refused, not followed into a new file", err)
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data")
	if got, _ := DefaultPath(); got != "/data/flowd/flowd.sqlite" {
		t.Errorf("got %q", got)
	}
	t.Setenv("XDG_DATA_HOME", "relative")
	t.Setenv("HOME", "/home/me")
	if got, _ := DefaultPath(); got != "/home/me/.local/share/flowd/flowd.sqlite" {
		t.Errorf("a relative XDG_DATA_HOME must be ignored, got %q", got)
	}
}

func chmodPrivate(path string) error { return os.Chmod(path, 0o600) }
