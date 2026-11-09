package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRepairSafety(t *testing.T) {
	ctx := context.Background()
	t.Run("competing process", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "day.sqlite")
		child := startStoreProcess(t, "hold", path)
		if _, err := Repair(ctx, path); !errors.Is(err, ErrLocked) {
			t.Errorf("want competing lock refusal, got %v", err)
		}
		child.Process.Kill()
		child.Wait()
	})
	t.Run("cancel and repeat", func(t *testing.T) {
		db := blockTestStore(t)
		path := db.Path()
		if _, err := db.Backup(ctx); err != nil {
			t.Fatal(err)
		}
		db.Close()
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := Repair(canceled, path); !errors.Is(err, context.Canceled) {
			t.Errorf("lost cancellation: %v", err)
		}
		first, err := Repair(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := os.ReadFile(filepath.Join(first, filepath.Base(path)))
		if err != nil {
			t.Fatal(err)
		}
		second, err := Repair(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		if second == first {
			t.Error("repeated repair reused a recovery location")
		}
		kept, err := os.ReadFile(filepath.Join(first, filepath.Base(path)))
		if err != nil || !bytes.Equal(saved, kept) {
			t.Error("repeated repair changed earlier originals")
		}
	})
	t.Run("unsupported journal alias", func(t *testing.T) {
		db := blockTestStore(t)
		path := db.Path()
		if _, err := db.Backup(ctx); err != nil {
			t.Fatal(err)
		}
		db.Close()
		before, _ := os.ReadFile(path)
		if err := os.Symlink(path+".bak", path+"-wal"); err != nil {
			t.Fatal(err)
		}
		if _, err := Repair(ctx, path); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("want unsafe path refusal, got %v", err)
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Error("unsupported path changed the original")
		}
	})
}

func TestInterruptedRepair(t *testing.T) {
	ctx := context.Background()
	db := blockTestStore(t)
	path := db.Path()
	if _, err := db.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	db.Close()
	replacement, err := prepareRepair(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := os.MkdirTemp(filepath.Dir(path), filepath.Base(path)+".recovery-")
	if err != nil {
		t.Fatal(err)
	}
	journal := []byte("preserved interrupted journal")
	if err := os.WriteFile(filepath.Join(recovery, filepath.Base(path)+"-wal"), journal, 0o600); err != nil {
		t.Fatal(err)
	}
	marker, err := json.Marshal(map[string]string{"recovery": recovery, "replacement": replacement, "stage": "moving"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".repair", marker, 0o600); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(ctx, path); err == nil {
		opened.Close()
		t.Error("normal open accepted an incomplete repair after journal movement")
	}
	resumed, err := Repair(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if resumed != recovery {
		t.Error("resume must retain the existing recovery location")
	}
	if _, err := os.Stat(path + ".repair"); !errors.Is(err, os.ErrNotExist) {
		t.Error("completed recovery must remove its marker")
	}
	kept, err := os.ReadFile(filepath.Join(recovery, filepath.Base(path)+"-wal"))
	if err != nil || !bytes.Equal(kept, journal) {
		t.Error("resume lost previously preserved journal")
	}
	if err := Inspect(ctx, path); err != nil {
		t.Fatal(err)
	}
}

func TestRepairPreservesOriginals(t *testing.T) {
	db := blockTestStore(t)
	ctx := context.Background()
	path := db.Path()
	if _, err := db.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	newer := exampleBlock(t, "bl_newer", "09:00", "10:00")
	saveBlockFixture(t, db, newer)
	db.Close()
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := Repair(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	preserved, err := os.ReadFile(filepath.Join(recovery, filepath.Base(path)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, preserved) {
		t.Error("repair must preserve exact original main bytes")
	}
	restored, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err := restored.BlockByID(ctx, newer.ID); !errors.Is(err, ErrBlockNotFound) {
		t.Error("explicit older snapshot restore must discard newer live rows")
	}
	if _, err := restored.DayByDate(ctx, exampleDay(t).Date); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(recovery)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Error("recovery directory must be private")
	}
}

func TestPrepareRepair(t *testing.T) {
	for _, kind := range []string{"valid", "missing", "corrupt", "newer"} {
		t.Run(kind, func(t *testing.T) {
			db := blockTestStore(t)
			ctx := context.Background()
			path := db.Path()
			if _, err := db.Backup(ctx); err != nil {
				t.Fatal(err)
			}
			db.Close()
			switch kind {
			case "missing":
				if err := os.Remove(path + ".bak"); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(path+".bak", []byte("bad backup"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "newer":
				raw, err := sql.Open("sqlite", generateDatabaseURL(path+".bak"))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := raw.Exec("PRAGMA user_version=2"); err != nil {
					t.Fatal(err)
				}
				raw.Close()
			}
			original, journal := []byte("unreadable original"), []byte("retained journal")
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path+"-wal", journal, 0o600); err != nil {
				t.Fatal(err)
			}
			replacement, err := prepareRepair(ctx, path)
			if kind == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				defer os.Remove(replacement)
				if err := inspectFile(ctx, replacement); err != nil {
					t.Fatal(err)
				}
			} else {
				want := ErrCorrupt
				if kind == "missing" {
					want = os.ErrNotExist
				}
				if kind == "newer" {
					want = ErrVersion
				}
				if !errors.Is(err, want) {
					t.Errorf("got %v, want %v", err, want)
				}
			}
			current, _ := os.ReadFile(path)
			log, _ := os.ReadFile(path + "-wal")
			if !bytes.Equal(current, original) || !bytes.Equal(log, journal) {
				t.Error("preparation must not change originals or journals")
			}
		})
	}
}
