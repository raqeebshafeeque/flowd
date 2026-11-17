package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// damagedWithBackup makes a database with one item, backs it up, then
// overwrites the live file with garbage.
func damagedWithBackup(t *testing.T) (path string, item domain.Item) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "flowd.sqlite")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, item = createTestItem(t, s)
	if _, err := s.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := os.WriteFile(path, []byte(strings.Repeat("broken ", 2000)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, item
}

func TestRepairRestoresTheBackupAndKeepsTheOriginal(t *testing.T) {
	path, item := damagedWithBackup(t)
	recovery, err := Repair(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal("the repaired database does not open:", err)
	}
	defer s.Close()
	if _, err := s.Item(ctx, item.ID); err != nil {
		t.Fatal("the restored item is missing:", err)
	}
	kept, err := os.ReadFile(filepath.Join(recovery, filepath.Base(path)))
	if err != nil || !strings.HasPrefix(string(kept), "broken") {
		t.Fatal("the damaged original was not kept", err)
	}
	if info, _ := os.Stat(recovery); info.Mode().Perm() != 0o700 {
		t.Fatal("the recovery folder is not private")
	}
	if _, err := os.Stat(path + repairSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the repair plan was left behind")
	}
}

func TestRepairWithoutAUsableBackupChangesNothing(t *testing.T) {
	path, _ := damagedWithBackup(t)
	if err := os.WriteFile(path+".bak", []byte("not a backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if _, err := Repair(ctx, path); err == nil {
		t.Fatal("a broken backup must be refused")
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("the live file was changed")
	}
	os.Remove(path + ".bak")
	if _, err := Repair(ctx, path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a missing backup must be reported", err)
	}
}

func TestInterruptedRepairBlocksOpeningAndCanBeFinished(t *testing.T) {
	for _, stage := range []string{"moving", "publishing"} {
		t.Run(stage, func(t *testing.T) {
			path, item := damagedWithBackup(t)
			plan, err := startRepair(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			if stage == "publishing" {
				if err := moveAside(path, plan.Recovery); err != nil {
					t.Fatal(err)
				}
				plan.Stage = "publishing"
				if err := writePlan(path, plan); err != nil {
					t.Fatal(err)
				}
			}
			// The process stops here. Normal use must refuse to touch the files.
			if _, err := Open(ctx, path); !errors.Is(err, ErrIncompleteRepair) {
				t.Fatal("open must refuse during an interrupted repair", err)
			}
			if err := Inspect(ctx, path); !errors.Is(err, ErrIncompleteRepair) {
				t.Fatal("inspect must report the interrupted repair", err)
			}
			recovery, err := Repair(ctx, path)
			if err != nil || recovery != plan.Recovery {
				t.Fatalf("finishing the repair: %q %v", recovery, err)
			}
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if _, err := s.Item(ctx, item.ID); err != nil {
				t.Fatal("the restored item is missing:", err)
			}
		})
	}
}
