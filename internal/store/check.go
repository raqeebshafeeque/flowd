package store

import (
	"context"
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func (s *Store) checkIntegrity(ctx context.Context, query string) error {
	var result string
	if err := s.db.QueryRowContext(ctx, query).Scan(&result); err != nil {
		return fmt.Errorf("check database integrity: %w", classify(err))
	}
	if result != "ok" {
		return fmt.Errorf("%w: %s", ErrCorrupt, result)
	}
	return nil
}

func (s *Store) checkSchema(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return classify(err)
	}
	if version != schemaVersion {
		return fmt.Errorf("%w: got %d, supported %d", ErrVersion, version, schemaVersion)
	}
	for _, table := range []struct{ name, columns string }{
		{"days", dayColumns}, {"blocks", blockColumns},
		{"block_events", "seq,block_id,from_status,to_status,occurred_at_utc,reason"},
	} {
		rows, err := s.db.QueryContext(ctx, "SELECT "+table.columns+" FROM "+table.name+" LIMIT 0")
		if err != nil {
			var driver *sqlite.Error
			if errors.As(err, &driver) && driver.Code()&0xff == sqlite3.SQLITE_ERROR {
				err = fmt.Errorf("%w: %w", ErrCorrupt, err)
			}
			return fmt.Errorf("inspect %s schema: %w", table.name, classify(err))
		}
		if err := rows.Close(); err != nil {
			return classify(err)
		}
	}
	return nil
}
