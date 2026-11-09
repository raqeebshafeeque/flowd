package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
)

const schemaVersion = 1

var ErrVersion = errors.New("unsupported database schema version")

//go:embed schema.sql
var schemaSQL string

// initializeSchema creates the tables when the database is new.
func (s *Store) initializeSchema(ctx context.Context) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schema setup: %w", err)
	}

	defer func() {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(
				err,
				fmt.Errorf("rollback schema setup: %w", rollbackErr),
			)
		}
	}()

	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	switch version {
	case schemaVersion:
		return nil
	case 0:
		// A new database has no schema yet.
	default:
		return fmt.Errorf("%w: got %d, supported %d", ErrVersion, version, schemaVersion)
	}

	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("create database tables: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema setup: %w", err)
	}

	return nil
}
