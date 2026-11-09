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
