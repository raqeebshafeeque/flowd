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

// SchemaVersion is the database layout this build reads and writes.
// Version 2 added items.carried_from. A version 1 file is upgraded the first
// time this build opens it to write, after a backup.
const SchemaVersion = 2

// versionOneColumns are what each table had in version 1, used to check an
// older file before upgrading it.
var versionOneColumns = map[string]string{
	"days":        dayColumns,
	"items":       "id,day_id,title,status,duration_minutes,started_at,finished_at,closed_at,archived,created_at,updated_at",
	"item_events": eventColumns,
}

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
// initialized. With upgradable, a well formed version 1 file passes too,
// because opening it to write upgrades it.
func checkSchema(ctx context.Context, db *sql.DB, upgradable bool) error {
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
	shape := tableColumns
	switch {
	case version == 1 && upgradable:
		shape = versionOneColumns
	case version != SchemaVersion:
		return fmt.Errorf("%w: found version %d, this build reads version %d", ErrVersion, version, SchemaVersion)
	}
	for table, columns := range shape {
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

// upgrade moves a version 1 file to this layout. It writes a backup first,
// the same one flowd backup makes, and then changes the file in one
// transaction, so a failure at any point leaves the file as it was.
func upgrade(ctx context.Context, db *sql.DB, path string) (err error) {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return classify(err)
	}
	if version != 1 {
		return nil
	}
	if err := checkSchema(ctx, db, true); err != nil {
		return err
	}
	if _, err := backup(ctx, db, path); err != nil {
		return fmt.Errorf("back up before upgrading: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return classify(err)
	}
	defer func() { err = errors.Join(err, rollback(tx)) }()
	for _, statement := range []string{
		"ALTER TABLE items ADD COLUMN carried_from TEXT REFERENCES items(id)",
		"CREATE INDEX items_by_origin ON items(carried_from) WHERE carried_from IS NOT NULL",
		"PRAGMA user_version = 2",
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("upgrade to version 2: %w", classify(err))
		}
	}
	return classify(tx.Commit())
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
