package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// SchemaVersion is the database layout this build reads and writes.
// Version 2 added items.carried_from and version 3 items.position. An older
// file is upgraded the first time this build opens it to write, after a
// backup, through every version in between.
const SchemaVersion = 3

// olderColumns are what each table had in the versions this build can
// upgrade, used to check an older file before upgrading it.
var olderColumns = map[int]map[string]string{
	1: {
		"days":        dayColumns,
		"items":       "id,day_id,title,status,duration_minutes,started_at,finished_at,closed_at,archived,created_at,updated_at",
		"item_events": eventColumns,
	},
	2: {
		"days":        dayColumns,
		"items":       itemColumns,
		"item_events": eventColumns,
	},
}

//go:embed schema.sql
var schemaSQL string

// tableColumns lists what each table must have, used to check the shape of
// an existing file before trusting it.
var tableColumns = map[string]string{
	"days":        dayColumns,
	"items":       itemColumns + ",position",
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
// initialized. With upgradable, a well formed older file passes too,
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
	case olderColumns[version] != nil && upgradable:
		shape = olderColumns[version]
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

// upgrade moves an older file to this layout, through every version in
// between. It writes a backup first, the same one flowd backup makes, and
// then makes every step in one transaction, so a failure at any point
// leaves the file as it was.
func upgrade(ctx context.Context, db *sql.DB, path string) (err error) {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return classify(err)
	}
	if olderColumns[version] == nil {
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
	if version < 2 {
		if err := upgradeToTwo(ctx, tx); err != nil {
			return fmt.Errorf("upgrade to version 2: %w", err)
		}
	}
	if err := upgradeToThree(ctx, tx); err != nil {
		return fmt.Errorf("upgrade to version 3: %w", err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		return classify(err)
	}
	return classify(tx.Commit())
}

// upgradeToTwo adds the item a copy was carried from.
func upgradeToTwo(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range []string{
		"ALTER TABLE items ADD COLUMN carried_from TEXT REFERENCES items(id)",
		"CREATE INDEX items_by_origin ON items(carried_from) WHERE carried_from IS NOT NULL",
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return classify(err)
		}
	}
	return nil
}

// upgradeToThree adds each item's place in its day. Every day keeps the
// order it showed before: started items first by their first start, then
// the rest in the order they were added.
func upgradeToThree(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "ALTER TABLE items ADD COLUMN position INTEGER NOT NULL DEFAULT 0"); err != nil {
		return classify(err)
	}
	days, err := tx.QueryContext(ctx, "SELECT id FROM days")
	if err != nil {
		return classify(err)
	}
	var ids []domain.ID
	for days.Next() {
		var id domain.ID
		if err := days.Scan(&id); err != nil {
			days.Close()
			return classify(err)
		}
		ids = append(ids, id)
	}
	if err := errors.Join(days.Err(), days.Close()); err != nil {
		return classify(err)
	}
	for _, dayID := range ids {
		items, err := readItems(ctx, tx, dayID)
		if err != nil {
			return classify(err)
		}
		domain.SortAsBefore(items)
		order := make([]domain.ID, len(items))
		for i, item := range items {
			order[i] = item.ID
		}
		if err := renumber(ctx, tx, dayID, order); err != nil {
			return classify(err)
		}
	}
	_, err = tx.ExecContext(ctx, "CREATE UNIQUE INDEX items_in_order ON items(day_id, position) WHERE archived=0")
	return classify(err)
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
