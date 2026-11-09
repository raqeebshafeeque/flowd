package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Copy a quiescent committed WAL snapshot without checkpointing the destination.
func privateSidecarFixture(t *testing.T) string {
	t.Helper()
	source := openTestStore(t)
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "flowd.sqlite")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		contents, err := os.ReadFile(source.Path() + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path+suffix, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path+suffix, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenTightensExistingSidecars(t *testing.T) {
	path := privateSidecarFixture(t)
	database, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, suffix := range []string{"-wal", "-shm"} {
		info, err := os.Stat(path + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s has mode %04o; want 0600", suffix, info.Mode().Perm())
		}
	}
}

func TestInspectRefusesNonPrivateSidecarsWithoutChangingThem(t *testing.T) {
	path := privateSidecarFixture(t)
	if err := Inspect(context.Background(), path); !errors.Is(err, ErrPermissions) {
		t.Errorf("non-private WAL inspection: %v; want permission refusal", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		info, err := os.Stat(path + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("read-only inspection changed %s mode", suffix)
		}
	}
}

func TestUnsafeSidecarsRefused(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(context.Context, string) error
	}{
		{"open", func(ctx context.Context, path string) error {
			database, err := Open(ctx, path)
			if database != nil {
				return errors.Join(err, database.Close())
			}
			return err
		}},
		{"inspect", Inspect},
	} {
		t.Run(operation.name, func(t *testing.T) {
			checkUnsafeSidecars(t, operation.run)
		})
	}
}

func checkUnsafeSidecars(t *testing.T, operation func(context.Context, string) error) {
	t.Helper()
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		for _, kind := range []string{"symlink", "hard link"} {
			t.Run(suffix+" "+kind, func(t *testing.T) {
				parent := t.TempDir()
				path := filepath.Join(parent, "flowd.sqlite")
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				other := filepath.Join(parent, "unrelated")
				if err := os.WriteFile(other, []byte("keep unrelated bytes"), 0o600); err != nil {
					t.Fatal(err)
				}
				var err error
				if kind == "symlink" {
					err = os.Symlink(other, path+suffix)
				} else {
					err = os.Link(other, path+suffix)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := operation(context.Background(), path); !errors.Is(err, ErrInvalidPath) {
					t.Errorf("unsafe sibling must be refused before SQLite: %v", err)
				}
				contents, err := os.ReadFile(other)
				if err != nil || string(contents) != "keep unrelated bytes" {
					t.Error("unsafe sidecar changed an unrelated file")
				}
			})
		}
	}
}
