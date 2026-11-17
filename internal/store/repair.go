package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrIncompleteRepair means a repair was interrupted. Running it again
// finishes it; until then the database is not opened.
var ErrIncompleteRepair = errors.New("a repair was interrupted, run flowd doctor --repair to finish it")

const repairSuffix = ".repair"

// repairPlan is written next to the database before any file moves, so an
// interrupted repair can always be finished.
type repairPlan struct {
	Recovery    string `json:"recovery"`
	Replacement string `json:"replacement"`
	Stage       string `json:"stage"` // "moving", then "publishing"
}

// Repair replaces the database with its backup. The current files are never
// deleted: they move into a private folder next to the database, whose path
// is returned. Changes made after the backup are lost from the live file but
// kept in that folder.
func Repair(ctx context.Context, path string) (recovery string, err error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	lock, err := lockFile(absolute)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	plan, err := readPlan(absolute)
	if errors.Is(err, os.ErrNotExist) {
		plan, err = startRepair(ctx, absolute)
	}
	if err != nil {
		return plan.Recovery, err
	}
	if plan.Stage == "moving" {
		if err := moveAside(absolute, plan.Recovery); err != nil {
			return plan.Recovery, err
		}
		plan.Stage = "publishing"
		if err := writePlan(absolute, plan); err != nil {
			return plan.Recovery, err
		}
	}
	return plan.Recovery, publish(ctx, absolute, plan)
}

// startRepair checks the backup, makes a private copy of it to publish and
// records the plan. Nothing in place has changed yet if this fails.
func startRepair(ctx context.Context, path string) (repairPlan, error) {
	backup := path + ".bak"
	if err := plainFile(backup); err != nil {
		return repairPlan{}, err
	}
	if _, err := os.Stat(backup); err != nil {
		return repairPlan{}, fmt.Errorf("no backup to restore: %w", err)
	}
	if err := checkFile(ctx, backup); err != nil {
		return repairPlan{}, fmt.Errorf("the backup cannot be used: %w", err)
	}
	replacement, err := tempFile(filepath.Dir(path), ".flowd-restore-*")
	if err != nil {
		return repairPlan{}, err
	}
	if err := copyFile(backup, replacement); err != nil {
		os.Remove(replacement)
		return repairPlan{}, err
	}
	if err := checkFile(ctx, replacement); err != nil {
		os.Remove(replacement)
		return repairPlan{}, err
	}
	recovery := fmt.Sprintf("%s.recovery-%s", path, time.Now().UTC().Format("20060102T150405Z"))
	if err := os.Mkdir(recovery, 0o700); err != nil {
		os.Remove(replacement)
		return repairPlan{}, err
	}
	plan := repairPlan{Recovery: recovery, Replacement: replacement, Stage: "moving"}
	if err := writePlan(path, plan); err != nil {
		os.Remove(replacement)
		os.Remove(recovery)
		return repairPlan{}, err
	}
	return plan, nil
}

// moveAside moves the database and its journal files into the recovery
// folder. A file already in the folder is never overwritten.
func moveAside(path, recovery string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal", ""} {
		source := path + suffix
		target := filepath.Join(recovery, filepath.Base(source))
		if err := plainFile(source); err != nil {
			return err
		}
		if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("%w: %s is already in the recovery folder", ErrIncompleteRepair, filepath.Base(source))
		}
		if err := os.Rename(source, target); err != nil {
			return err
		}
	}
	return errors.Join(syncDir(recovery), syncDir(filepath.Dir(path)))
}

// publish puts the prepared copy in place, checks it and removes the plan. If
// the copy was already moved before an interruption, the live file is
// checked instead.
func publish(ctx context.Context, path string, plan repairPlan) error {
	if _, err := os.Lstat(plan.Replacement); err == nil {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("%w: a database appeared before the backup was restored", ErrIncompleteRepair)
		}
		if err := os.Rename(plan.Replacement, path); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := checkFile(ctx, path); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.Remove(path + repairSuffix); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func readPlan(path string) (repairPlan, error) {
	raw, err := os.ReadFile(path + repairSuffix)
	if err != nil {
		return repairPlan{}, err
	}
	var plan repairPlan
	if err := json.Unmarshal(raw, &plan); err != nil || plan.Recovery == "" || plan.Replacement == "" || (plan.Stage != "moving" && plan.Stage != "publishing") {
		return repairPlan{}, fmt.Errorf("%w: the repair plan is unreadable", ErrIncompleteRepair)
	}
	return plan, nil
}

// writePlan replaces the plan file atomically and makes sure it is on disk.
func writePlan(path string, plan repairPlan) error {
	raw, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	temporary := path + repairSuffix + ".new"
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return err
	}
	if err := syncFile(temporary); err != nil {
		return err
	}
	if err := os.Rename(temporary, path+repairSuffix); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}
