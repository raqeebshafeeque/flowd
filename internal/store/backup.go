package store

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

// Backup publishes a SQLite snapshot, including committed WAL contents.
func (s *Store) Backup(ctx context.Context) (string, error) {
	return s.backup(ctx, os.Rename, syncDirectory)
}

func (s *Store) backup(ctx context.Context, rename func(string, string) error, syncParent func(string) error) (_ string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("backup database %q: %w", s.path, classify(err))
		}
	}()
	destination := s.path + ".bak"
	if err := safeSibling(destination); err != nil {
		return "", err
	}
	temporary, err := snapshot(ctx, s, filepath.Dir(s.path))
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := rename(temporary, destination); err != nil {
		return "", err
	}
	if err := syncParent(filepath.Dir(destination)); err != nil {
		return destination, fmt.Errorf("snapshot published but directory sync failed: %w", err)
	}
	return destination, nil
}

func snapshot(ctx context.Context, s *Store, directory string) (_ string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, ".flowd-snapshot-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	defer func() {
		if err != nil {
			os.Remove(path)
		}
	}()
	if err := file.Close(); err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return "", classify(err)
	}
	file, err = os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	err = errors.Join(file.Sync(), file.Close())
	if err != nil {
		return "", err
	}
	return path, nil
}

func syncDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

// Managed siblings are not aliases or alternate file types.
func safeSibling(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: unsupported managed file %q", ErrInvalidPath, path)
	}
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return err
	}
	if stat.Nlink != 1 {
		return fmt.Errorf("%w: hard-linked managed file %q", ErrInvalidPath, path)
	}
	return nil
}
