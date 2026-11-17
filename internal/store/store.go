package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	ErrCorrupt     = errors.New("the database is damaged")
	ErrVersion     = errors.New("the database was made by an unsupported version")
	ErrLocked      = errors.New("the database is in use by another flowd operation")
	ErrDiskFull    = errors.New("the disk is full")
	ErrInvalidPath = errors.New("the database path is not a plain private file")
	ErrPermissions = errors.New("the database is not private to you")
	ErrNotFound    = errors.New("not found")
	ErrStale       = errors.New("the item changed since it was read")
	ErrInvalidSave = errors.New("the change does not match its event")
)

// Store owns one open database file until Close.
type Store struct {
	db   *sql.DB
	path string
	lock *os.File
}

// DefaultPath is $XDG_DATA_HOME/flowd/flowd.sqlite, or
// ~/.local/share/flowd/flowd.sqlite when that variable is not an absolute
// path. It only works out the name and creates nothing.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "flowd", "flowd.sqlite"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find the home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "flowd", "flowd.sqlite"), nil
}

// Open takes ownership of the database at path, creating it if it does not
// exist. An existing file is checked for damage and for the right schema
// before anything is written to it.
func Open(ctx context.Context, path string) (_ *Store, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absolute, err := resolvePath(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	lock, err := lockFile(absolute)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", absolute, err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, lock.Close())
		}
	}()
	if _, err := os.Stat(absolute + repairSuffix); err == nil {
		return nil, fmt.Errorf("open %q: %w", absolute, ErrIncompleteRepair)
	}
	if err := inspectExisting(ctx, absolute); err != nil {
		return nil, fmt.Errorf("open %q: %w", absolute, err)
	}
	db, err := connect(ctx, absolute, false)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", absolute, err)
	}
	if err := initialize(ctx, db); err != nil {
		return nil, errors.Join(fmt.Errorf("open %q: %w", absolute, err), db.Close())
	}
	return &Store{db: db, path: absolute, lock: lock}, nil
}

// Path is the absolute path of the database file.
func (s *Store) Path() string { return s.path }

// Close releases the database and its lock.
func (s *Store) Close() error {
	return errors.Join(s.db.Close(), s.lock.Close())
}

// resolvePath makes the parent directory if needed and refuses anything but a
// plain file with a single name: no directories, devices or hard links.
// A symbolic link to such a file is followed.
func resolvePath(path string) (string, error) {
	if path == "" {
		return "", ErrInvalidPath
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := plainFile(absolute + suffix); err != nil {
			return "", err
		}
	}
	return absolute, nil
}

// plainFile refuses a path that exists but is not a regular file with one name.
func plainFile(path string) error {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); errors.Is(err, unix.ENOENT) {
		return nil
	} else if err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return fmt.Errorf("%w: %s", ErrInvalidPath, path)
	}
	return nil
}

// lockFile takes the lock that stops two flowd operations using the file at
// once. The lock file is never deleted, because another process may be
// waiting on it.
func lockFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if err := plainFile(path + ".lock"); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			err = ErrLocked
		}
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

// inspectExisting checks a file that is already there, read only, before it
// is opened for writing. An empty or damaged file is refused, never replaced.
func inspectExisting(ctx context.Context, path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("%w: the file is empty", ErrCorrupt)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: mode is %04o, expected 0600", ErrPermissions, info.Mode().Perm())
	}
	db, err := connect(ctx, path, true)
	if err != nil {
		return err
	}
	err = checkIntegrity(ctx, db)
	if err == nil {
		err = checkSchema(ctx, db)
	}
	return errors.Join(err, db.Close())
}

// connect opens one connection. Writable connections use WAL and wait for
// each save to reach the disk.
func connect(ctx context.Context, path string, readOnly bool) (*sql.DB, error) {
	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	if readOnly {
		query.Set("mode", "ro")
	} else {
		query.Add("_pragma", "journal_mode(WAL)")
		query.Add("_pragma", "synchronous(FULL)")
		if err := createPrivate(path); err != nil {
			return nil, err
		}
	}
	source := url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}
	db, err := sql.Open("sqlite", source.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(classify(err), db.Close())
	}
	return db, nil
}

// createPrivate makes a new database file readable only by its owner.
func createPrivate(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return file.Close()
}

// transact runs work in one transaction and always ends it.
func (s *Store) transact(ctx context.Context, work func(*sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return classify(err)
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	if err := work(tx); err != nil {
		return classify(err)
	}
	return classify(tx.Commit())
}

// classify adds a stable category to driver and system errors while keeping
// the original cause.
func classify(err error) error {
	if err == nil || errors.Is(err, ErrCorrupt) || errors.Is(err, ErrDiskFull) || errors.Is(err, ErrLocked) {
		return err
	}
	var category error
	var driver *sqlite.Error
	var parse *time.ParseError
	switch {
	case errors.Is(err, unix.ENOSPC), errors.Is(err, unix.EDQUOT):
		category = ErrDiskFull
	case errors.As(err, &parse):
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
	if category == nil {
		return err
	}
	return fmt.Errorf("%w: %w", category, err)
}
