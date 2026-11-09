package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Inspect observes an existing database without initializing or tightening it.
func Inspect(ctx context.Context, path string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err = resolvePath(path, false)
	if err != nil {
		return err
	}
	// Missing data should not create even a coordination file, unless a repair
	// marker explains why the main file is temporarily absent.
	if _, err := os.Stat(path); err != nil && checkRepair(path) == nil {
		return err
	}
	lock, err := lockDatabase(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	if err := checkRepair(path); err != nil {
		return err
	}
	if err := privateFile(path); err != nil {
		return err
	}
	return inspectFile(ctx, path)
}

func privateFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrInvalidPath
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("%w: %q has mode %04o; expected 0600", ErrPermissions, path, info.Mode().Perm())
	}
	if err := unix.Access(path, unix.R_OK|unix.W_OK); err != nil {
		return err
	}
	return unix.Access(filepath.Dir(path), unix.W_OK)
}

func readOnly(ctx context.Context, path string) (_ *Store, err error) {
	parameters := url.Values{"mode": {"ro"}, "_pragma": {"foreign_keys(1)"}}
	source := url.URL{Scheme: "file", Path: path, RawQuery: parameters.Encode()}
	db, err := sql.Open("sqlite", source.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(classify(err), db.Close())
	}
	return &Store{db: db, path: path}, nil
}

func inspectFile(ctx context.Context, path string) (err error) {
	db, err := readOnly(ctx, path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if err := db.checkIntegrity(ctx, "PRAGMA integrity_check"); err != nil {
		return err
	}
	if err := db.checkSchema(ctx); err != nil {
		return err
	}
	rows, err := db.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return classify(err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("%w: a foreign key references a missing row", ErrCorrupt)
	}
	return classify(rows.Err())
}
