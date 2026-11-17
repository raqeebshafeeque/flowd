package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// SchemaVersion is the only database layout this build reads and writes.
const SchemaVersion = 1

//go:embed schema.sql
var schemaSQL string

// tableColumns lists what each table must have, used to check the shape of
// an existing file before trusting it.
var tableColumns = map[string]string{
	"days":        dayColumns,
	"items":       itemColumns,
	"item_events": eventColumns,
}

// initialize creates the tables in a brand new file. A file that already has
// other tables, or another version, is never touched.
func initialize(ctx context.Context, db *sql.DB) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return classify(err)
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	var version, objects int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return classify(err)
	}
	if version == SchemaVersion {
		return nil
	}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'").Scan(&objects); err != nil {
		return classify(err)
	}
	if version != 0 || objects != 0 {
		return fmt.Errorf("%w: found version %d, this build reads version %d", ErrVersion, version, SchemaVersion)
	}
	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("create tables: %w", classify(err))
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		return classify(err)
	}
	return classify(tx.Commit())
}

// checkSchema refuses a file whose version or tables are not what this build
// expects. A brand new empty file (version 0, no tables) passes so it can be
// initialized.
func checkSchema(ctx context.Context, db *sql.DB) error {
	var version, objects int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return classify(err)
	}
	if version == 0 {
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'").Scan(&objects); err != nil {
			return classify(err)
		}
		if objects == 0 {
			return nil
		}
	}
	if version != SchemaVersion {
		return fmt.Errorf("%w: found version %d, this build reads version %d", ErrVersion, version, SchemaVersion)
	}
	for table, columns := range tableColumns {
		rows, err := db.QueryContext(ctx, "SELECT "+columns+" FROM "+table+" LIMIT 0")
		if err != nil {
			var driver *sqlite.Error
			if errors.As(err, &driver) && driver.Code()&0xff == sqlite3.SQLITE_ERROR {
				err = fmt.Errorf("%w: table %s has the wrong shape: %w", ErrCorrupt, table, err)
			}
			return classify(err)
		}
		if err := rows.Close(); err != nil {
			return classify(err)
		}
	}
	return nil
}

// checkIntegrity asks SQLite to look for damage in the file.
func checkIntegrity(ctx context.Context, db *sql.DB) error {
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&result); err != nil {
		return classify(err)
	}
	if result != "ok" {
		return fmt.Errorf("%w: %s", ErrCorrupt, result)
	}
	return nil
}

func rollback(tx *sql.Tx) error {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return err
	}
	return nil
}
