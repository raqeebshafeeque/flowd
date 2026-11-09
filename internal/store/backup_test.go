package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"golang.org/x/sys/unix"
)

func TestBackupDestinationOwnership(t *testing.T) {
	db := blockTestStore(t)
	ctx := context.Background()
	path, err := db.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	held, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Backup(ctx); !errors.Is(err, ErrLocked) {
		t.Errorf("backup overwrote an owned snapshot: %v", err)
	}
	held.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+"-wal", []byte("unrecovered snapshot journal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Backup(ctx); !errors.Is(err, ErrLocked) {
		t.Errorf("backup must not publish under a stale snapshot journal: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("refused publication changed the previous snapshot")
	}
}

func TestBackupFailures(t *testing.T) {
	db := blockTestStore(t)
	ctx := context.Background()
	path, err := db.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := db.Backup(canceled); !errors.Is(err, context.Canceled) {
		t.Errorf("lost cancellation: %v", err)
	}
	cause := &os.PathError{Op: "rename", Path: path, Err: unix.ENOSPC}
	if _, err := db.backup(ctx, func(string, string) error { return cause }, syncDirectory); !errors.Is(err, ErrDiskFull) || !errors.Is(err, cause) {
		t.Errorf("lost publication cause: %v", err)
	}
	retained, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(old, retained) {
		t.Error("pre-publication failure replaced the prior snapshot")
	}
	if _, err := db.backup(ctx, os.Rename, func(string) error { return cause }); !errors.Is(err, cause) || !strings.Contains(err.Error(), "published") {
		t.Errorf("post-publication failure must disclose publication: %v", err)
	}
	if err := Inspect(ctx, path); err != nil {
		t.Errorf("published snapshot should remain valid: %v", err)
	}
	if _, err := db.DayByDate(ctx, exampleDay(t).Date); err != nil {
		t.Errorf("live store should remain usable: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".flowd-snapshot-*"))
	if err != nil || len(files) != 0 {
		t.Error("temporary snapshots were not cleaned")
	}
}

func TestBackupIncludesWAL(t *testing.T) {
	db := blockTestStore(t)
	ctx := context.Background()
	if _, err := db.db.Exec("PRAGMA wal_autocheckpoint=0"); err != nil {
		t.Fatal(err)
	}
	block := exampleBlock(t, "bl_journal", "09:00", "10:00")
	if err := db.CreateBlock(ctx, block, exampleEvent(block, block.Status, block.CreatedAt, domain.ReasonUser)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(db.Path() + "-wal")
	if err != nil || info.Size() == 0 {
		t.Fatal("fixture must retain a committed WAL")
	}
	path, err := db.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if path != db.Path()+".bak" {
		t.Errorf("got backup path %q", path)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Error("backup must be private")
	}
	snapshot, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	if got, err := snapshot.BlockByID(ctx, block.ID); err != nil || got.ID != block.ID {
		t.Fatalf("snapshot lost journal block: %v", err)
	}
	events, err := snapshot.EventsByBlock(ctx, block.ID)
	if err != nil || len(events) != 1 {
		t.Errorf("snapshot lost matching history: %v", err)
	}
}
