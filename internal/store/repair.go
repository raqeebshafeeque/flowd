package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Repair replaces the database with its backup. The backup is checked first.
// The current files are never deleted: they move into a private folder next
// to the database, whose path is returned, so changes made after the backup
// can still be recovered by hand.
func Repair(ctx context.Context, path string) (recovery string, err error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	lock, err := lockFile(absolute)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	if _, err := os.Stat(absolute + ".bak"); err != nil {
		return "", fmt.Errorf("there is no backup: %w", err)
	}
	if err := checkFile(ctx, absolute+".bak"); err != nil {
		return "", fmt.Errorf("the backup cannot be used: %w", err)
	}
	if recovery, err = moveAside(absolute); err != nil {
		return recovery, err
	}
	if err := copyFile(absolute+".bak", absolute); err != nil {
		return recovery, err
	}
	return recovery, syncDir(filepath.Dir(absolute))
}

// ErrNothingToReset means there is no database to reset.
var ErrNothingToReset = errors.New("there is no database to reset")

// Reset leaves an empty planner behind: the current files move into a
// private recovery folder next to the database, whose path is returned, and
// the next open starts a new file. Nothing is deleted, and the backup stays
// where it is.
func Reset(ctx context.Context, path string) (recovery string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(absolute); errors.Is(err, os.ErrNotExist) {
		return "", ErrNothingToReset
	}
	lock, err := lockFile(absolute)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	if recovery, err = moveAside(absolute); err != nil {
		return recovery, err
	}
	return recovery, syncDir(filepath.Dir(absolute))
}

// moveAside moves the database and its SQLite side files into a new private
// folder next to it, named after the current time, and returns its path.
func moveAside(absolute string) (string, error) {
	base := absolute + ".recovery-" + time.Now().UTC().Format("20060102T150405Z")
	recovery := base
	for n := 2; ; n++ {
		err := os.Mkdir(recovery, 0o700)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) || n > 100 {
			return "", err
		}
		recovery = fmt.Sprintf("%s-%d", base, n)
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		from := absolute + suffix
		if _, err := os.Lstat(from); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := os.Rename(from, filepath.Join(recovery, filepath.Base(from))); err != nil {
			return recovery, err
		}
	}
	return recovery, nil
}
