package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// Backup writes a consistent copy of the database next to it as
// "flowd.sqlite.bak", including changes still waiting in the WAL file. The new
// backup replaces the old one only after it is complete and on disk.
func (s *Store) Backup(ctx context.Context) (string, error) {
	destination := s.path + ".bak"
	if err := plainFile(destination); err != nil {
		return "", err
	}
	temporary, err := tempFile(filepath.Dir(s.path), ".flowd-backup-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary)
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", temporary); err != nil {
		return "", fmt.Errorf("back up %q: %w", s.path, classify(err))
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		return "", err
	}
	if err := syncFile(temporary); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, destination); err != nil {
		return "", err
	}
	if err := syncDir(filepath.Dir(destination)); err != nil {
		return destination, fmt.Errorf("the backup was written but the folder could not be synced: %w", err)
	}
	return destination, nil
}

// Inspect checks a database without changing it and returns the first
// problem it finds: a missing or shared file, an unfinished repair, another
// flowd using it, damage, or an unknown schema.
func Inspect(ctx context.Context, path string) (err error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absolute + repairSuffix); err == nil {
		return ErrIncompleteRepair
	}
	if err := plainFile(absolute); err != nil {
		return err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: mode is %04o, expected 0600", ErrPermissions, info.Mode().Perm())
	}
	lock, err := lockFile(absolute)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	return checkFile(ctx, absolute)
}

// checkFile runs the full integrity check, the schema check and the foreign
// key check on a file, read only.
func checkFile(ctx context.Context, path string) error {
	db, err := connect(ctx, path, true)
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check(1)").Scan(&result); err != nil {
		return classify(err)
	}
	if result != "ok" {
		return fmt.Errorf("%w: %s", ErrCorrupt, result)
	}
	if err := checkSchema(ctx, db); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return classify(err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("%w: a row points at something that does not exist", ErrCorrupt)
	}
	return classify(rows.Err())
}

// tempFile creates an empty private file and returns its name.
func tempFile(dir, pattern string) (string, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	name := file.Name()
	if err := errors.Join(file.Chmod(0o600), file.Close()); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

// copyFile copies src to a new private file dst and syncs it.
func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func syncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

// UnsafeTitles lists the items whose saved title, now or in their history,
// holds hidden or direction-changing characters. Such titles are shown with
// placeholders, so this is a warning, not damage. Nothing is changed.
func UnsafeTitles(ctx context.Context, path string) (ids []domain.ID, err error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	db, err := connect(ctx, absolute, true)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	rows, err := db.QueryContext(ctx, `SELECT id, title FROM items
 UNION SELECT item_id, json_extract(after_json, '$.title') FROM item_events
 UNION SELECT item_id, json_extract(before_json, '$.title') FROM item_events WHERE before_json IS NOT NULL`)
	if err != nil {
		return nil, classify(err)
	}
	defer rows.Close()
	seen := map[domain.ID]bool{}
	for rows.Next() {
		var id domain.ID
		var title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, classify(err)
		}
		if domain.CheckText(title) != nil && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids, classify(rows.Err())
}
