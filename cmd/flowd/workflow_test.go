package main

import (
	"encoding/json"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"os/exec"
	"path/filepath"
	"testing"
)

func binaryCommand(t *testing.T, binary, path string, args ...string) []byte {
	t.Helper()
	flags := []string{"--db", path, "--date", "2026-10-14", "--timezone", "UTC", "--json"}
	output, err := exec.Command(binary, append(flags, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("command %v failed: %v; %s", args, err, output)
	}
	return output
}

func TestExecutableTracking(t *testing.T) {
	binary := buildExecutable(t)
	path := filepath.Join(t.TempDir(), "track.sqlite")
	binaryCommand(t, binary, path, "add", "09:00", "10:00", "Study")
	binaryCommand(t, binary, path, "add", "11:00", "12:00", "Code")
	binaryCommand(t, binary, path, "start", "1")
	binaryCommand(t, binary, path, "start", "2")
	binaryCommand(t, binary, path, "finish", "2")
	var events []domain.BlockEvent
	if err := json.Unmarshal(binaryCommand(t, binary, path, "events", "2"), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[2].ToStatus != domain.Done {
		t.Error("history should survive start/switch/finish processes")
	}
	var view struct{ Blocks []domain.Block }
	if err := json.Unmarshal(binaryCommand(t, binary, path, "list"), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Blocks) != 2 || view.Blocks[0].Status != domain.Paused || view.Blocks[1].Status != domain.Done {
		t.Error("switch must durably pause the previous runner")
	}
}

func TestExecutablePlanning(t *testing.T) {
	binary := buildExecutable(t)
	path := filepath.Join(t.TempDir(), "plan.sqlite")
	var created, edited, archived domain.Block
	for _, step := range []struct {
		args   []string
		result *domain.Block
	}{
		{[]string{"add", "09:00", "10:00", "Study"}, &created},
		{[]string{"edit", "--title", "Deep study", "1"}, &edited},
		{[]string{"delete", "1"}, &archived},
	} {
		if err := json.Unmarshal(binaryCommand(t, binary, path, step.args...), step.result); err != nil {
			t.Fatal(err)
		}
	}
	if created.ID == "" || created.ID != edited.ID || edited.Title != "Deep study" || !archived.Archived {
		t.Error("planning state must survive independent processes")
	}
}
