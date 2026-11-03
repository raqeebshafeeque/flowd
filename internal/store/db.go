package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (_ *Store, err error) {
	dbURL := generateDatabaseURL(path)

	db, err := sql.Open("sqlite", dbURL)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}

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
		err = fmt.Errorf("ping database %q: %w", path, err)
		return nil, err
	}

	store := &Store{db: db}
	if err = store.initializeSchema(ctx); err != nil {
		err = fmt.Errorf("initialize schema %q: %w", path, err)
		return nil, err
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

func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close store: %w", err)
	}
	return nil
}
