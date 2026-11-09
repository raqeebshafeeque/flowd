package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, test := range []struct{ name, xdg, want string }{
		{"absolute data home", filepath.Join(home, "data"), filepath.Join(home, "data", "flowd", "flowd.sqlite")},
		{"home fallback", "", filepath.Join(home, ".local", "share", "flowd", "flowd.sqlite")},
		{"relative data home", "relative", filepath.Join(home, ".local", "share", "flowd", "flowd.sqlite")},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", test.xdg)
			got, err := DefaultPath()
			if err != nil || got != test.want {
				t.Errorf("got path %q and %v, want %q", got, err, test.want)
			}
			if _, err := os.Stat(filepath.Dir(got)); !os.IsNotExist(err) {
				t.Error("path resolution should not create directories")
			}
		})
	}
}

func TestPrivateDatabasePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new-data", "flowd.sqlite")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("normal open should create its private directory: %v", err)
	}
	defer store.Close()
	for _, test := range []struct {
		path string
		mode os.FileMode
	}{{path, 0o600}, {filepath.Dir(path), 0o700}} {
		info, err := os.Stat(test.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != test.mode {
			t.Errorf("got mode %04o, want %04o", info.Mode().Perm(), test.mode)
		}
	}
}

func TestEmptyPathRefused(t *testing.T) {
	store, err := Open(context.Background(), "")
	if store != nil {
		_ = store.Close()
	}
	if err == nil {
		t.Fatal("empty path must not open a temporary or in-memory database")
	}
}

func TestExistingPermissions(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "flowd.sqlite")
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	file, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if file.Mode().Perm() != 0o600 {
		t.Error("normal open should tighten database mode")
	}
	if directory.Mode().Perm() != 0o755 {
		t.Error("existing parent permissions must not change")
	}
}
