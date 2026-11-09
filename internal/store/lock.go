package store

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

var ErrLocked = errors.New("database is in use by another flowd operation")

// A stable sibling inode also coordinates replacement of an unreadable database.
// Never unlink it: another process could still be locking the old inode.
func lockDatabase(path string) (*os.File, error) {
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open database lock: %w", err)
	}
	var info unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &info); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if info.Nlink != 1 || info.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, errors.Join(ErrInvalidPath, file.Close())
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			err = fmt.Errorf("%w: %w", ErrLocked, err)
		}
		return nil, errors.Join(err, file.Close())
	}
	if err := file.Chmod(0o600); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}
