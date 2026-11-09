package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepairCommand(t *testing.T) {
	f := newCommandFixture(t)
	block := f.saved("add", "09:00", "10:00", "Study")
	f.ok("backup")
	if err := os.WriteFile(f.path, []byte("damaged live database"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.fail("ERR_CORRUPT", 1, "list")
	var result struct {
		Recovery string
		OK       bool
	}
	if err := json.Unmarshal([]byte(f.ok("--json", "doctor", "--repair")), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Recovery == "" {
		t.Error("repair must identify retained originals")
	}
	if _, err := os.Stat(filepath.Join(result.Recovery, filepath.Base(f.path))); err != nil {
		t.Fatal(err)
	}
	if got := f.view().Blocks; len(got) != 1 || got[0].ID != block.ID {
		t.Error("repair lost backed-up work")
	}
	f.ok("doctor")
}

func TestMaintenanceCommands(t *testing.T) {
	f := newCommandFixture(t)
	f.fail("ERR_NOT_FOUND", 1, "doctor")
	if _, err := os.Stat(f.path); !os.IsNotExist(err) {
		t.Error("doctor created missing database")
	}
	block := f.saved("add", "09:00", "10:00", "Study")
	if out := f.ok("doctor"); !strings.Contains(out, "OK") {
		t.Error("healthy doctor must say OK")
	}
	var result struct {
		Path string
		OK   bool
	}
	if err := json.Unmarshal([]byte(f.ok("--json", "doctor")), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Path != f.path {
		t.Error("JSON doctor must identify the checked database")
	}
	if err := json.Unmarshal([]byte(f.ok("--json", "backup")), &result); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != canonical+".bak" {
		t.Error("JSON backup must identify the published snapshot")
	}
	snapshot := &commandFixture{t: t, path: f.path + ".bak", now: f.now}
	if got := snapshot.view().Blocks; len(got) != 1 || got[0].ID != block.ID {
		t.Error("command backup must retain blocks")
	}
	if err := os.Chmod(f.path, 0o644); err != nil {
		t.Fatal(err)
	}
	f.fail("ERR_PERMISSION", 1, "doctor")
	info, _ := os.Stat(f.path)
	if info.Mode().Perm() != 0o644 {
		t.Error("doctor repaired permissions without consent")
	}
	missing := filepath.Join(t.TempDir(), "missing", "day.sqlite")
	other := &commandFixture{t: t, path: missing, now: f.now}
	other.fail("ERR_NOT_FOUND", 1, "doctor")
	if _, err := os.Stat(filepath.Dir(missing)); !os.IsNotExist(err) {
		t.Error("doctor created a missing parent")
	}
}
