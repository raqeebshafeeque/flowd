package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
	lock *os.File
}

func Open(ctx context.Context, path string) (_ *Store, err error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	absolute, err := resolvePath(path, true)
	if err != nil {
		return nil, fmt.Errorf("resolve database %q: %w", path, err)
	}
	lock, err := lockDatabase(absolute)
	if err != nil {
		return nil, fmt.Errorf("lock database %q: %w", path, err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, lock.Close())
		}
	}()
	if err := checkRepair(absolute); err != nil {
		return nil, err
	}
	if err := prepareSidecars(absolute); err != nil {
		return nil, fmt.Errorf("prepare database sidecars %q: %w", path, err)
	}
	if err := prepareDatabaseFile(absolute); err != nil {
		return nil, fmt.Errorf("prepare database %q: %w", path, err)
	}
	dbURL := generateDatabaseURL(absolute)

	db, err := sql.Open("sqlite", dbURL)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer func() {
		if err != nil {
			if closeErr := db.Close(); closeErr != nil {
				err = errors.Join(
					err, fmt.Errorf("close database %q: %w", path, closeErr),
				)
			}
		}
	}()

	if err = db.PingContext(ctx); err != nil {
		err = fmt.Errorf("ping database %q: %w", path, classify(err))
		return nil, err
	}

	store := &Store{db: db, path: absolute, lock: lock}
	if err := store.checkIntegrity(ctx, "PRAGMA quick_check(1)"); err != nil {
		return nil, fmt.Errorf("check database %q: %w", path, err)
	}
	if err = store.initializeSchema(ctx); err != nil {
		err = fmt.Errorf("initialize schema %q: %w", path, classify(err))
		return nil, err
	}

	if err := store.checkSchema(ctx); err != nil {
		return nil, fmt.Errorf("inspect database %q: %w", path, err)
	}
	return store, nil
}

func generateDatabaseURL(path string) string {
	parameters := url.Values{}
	parameters.Set("_pragma", "foreign_keys(1)")
	parameters.Add("_pragma", "journal_mode(WAL)")
	parameters.Add("_pragma", "synchronous(FULL)")

	databaseURL := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: parameters.Encode(),
	}
	return databaseURL.String()
}

// Path identifies the canonical local database file.
func (s *Store) Path() string { return s.path }

func (s *Store) Close() error {
	err := s.db.Close()
	if s.lock != nil {
		if closeErr := s.lock.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
	}
	if err != nil {
		return fmt.Errorf("close store: %w", classify(err))
	}
	return nil
}
