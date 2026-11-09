package store

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/unix"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	ErrCorrupt     = errors.New("database is damaged")
	ErrDiskFull    = errors.New("disk is full")
	ErrPermissions = errors.New("database permissions are not private")
)

// classify keeps both a stable category and the original driver/OS/parse cause.
func classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrCorrupt) || errors.Is(err, ErrDiskFull) || errors.Is(err, ErrLocked) {
		return err
	}
	var category error
	var driver *sqlite.Error
	var malformed *time.ParseError
	switch {
	case errors.Is(err, unix.ENOSPC), errors.Is(err, unix.EDQUOT):
		category = ErrDiskFull
	case errors.As(err, &malformed):
		category = ErrCorrupt
	case errors.As(err, &driver):
		switch driver.Code() & 0xff {
		case sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED:
			category = ErrLocked
		case sqlite3.SQLITE_CORRUPT, sqlite3.SQLITE_NOTADB:
			category = ErrCorrupt
		case sqlite3.SQLITE_FULL:
			category = ErrDiskFull
		}
	}
	if category != nil {
		return fmt.Errorf("%w: %w", category, err)
	}
	return err
}
