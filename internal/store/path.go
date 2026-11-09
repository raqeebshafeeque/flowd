package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var ErrInvalidPath = errors.New("invalid database path")

// DefaultPath only resolves a name; it never creates directories or files.
func DefaultPath() (string, error) {
	if directory := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(directory) {
		return filepath.Join(directory, "flowd", "flowd.sqlite"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "flowd", "flowd.sqlite"), nil
}

func resolvePath(path string, create bool) (string, error) {
	if path == "" {
		return "", ErrInvalidPath
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if create {
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			return "", err
		}
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	absolute = filepath.Join(parent, filepath.Base(absolute))
	info, err := os.Lstat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		return absolute, nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		absolute, err = filepath.EvalSymlinks(absolute)
		if err != nil {
			return "", err
		}
		info, err = os.Stat(absolute)
		if err != nil {
			return "", err
		}
	}
	if !info.Mode().IsRegular() {
		return "", &os.PathError{Op: "resolve database", Path: absolute, Err: ErrInvalidPath}
	}
	var stat unix.Stat_t
	if err := unix.Stat(absolute, &stat); err != nil {
		return "", err
	}
	if stat.Nlink != 1 {
		return "", fmt.Errorf("hard-linked database %q: %w", absolute, ErrInvalidPath)
	}
	return absolute, nil
}

func prepareDatabaseFile(path string) error {
	// Never open and close an extra descriptor for an existing database: POSIX
	// record locks can be disturbed by closing a descriptor to the same inode.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		return os.Chmod(path, 0o600)
	}
	if err != nil {
		return err
	}
	return file.Close()
}
