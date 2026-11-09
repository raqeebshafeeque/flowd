package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

func TestCompletePlannerWorkflow(t *testing.T) {
	binary := buildExecutable(t)
	path := filepath.Join(t.TempDir(), "complete.sqlite")
	saved := func(args ...string) domain.Block {
		t.Helper()
		var block domain.Block
		if err := json.Unmarshal(binaryCommand(t, binary, path, args...), &block); err != nil {
			t.Fatal(err)
		}
		return block
	}
	first := saved("add", "09:00", "10:00", "Study")
	second := saved("add", "11:00", "12:00", "Code")
	saved("edit", "--title", "Deep study", string(first.ID))
	saved("start", string(first.ID))
	saved("start", string(second.ID))
	saved("resume", string(first.ID))
	saved("extend", string(first.ID))
	saved("finish", string(first.ID))
	copied := saved("copy", string(second.ID))
	var review struct {
		Done, Unfinished int
		Progress         *int
	}
	if err := json.Unmarshal(binaryCommand(t, binary, path, "review"), &review); err != nil {
		t.Fatal(err)
	}
	if review.Done != 1 || review.Unfinished != 1 || review.Progress == nil || *review.Progress != 50 {
		t.Error("review lost execution state")
	}
	saved("delete", string(first.ID))
	var events []domain.BlockEvent
	if err := json.Unmarshal(binaryCommand(t, binary, path, "events", string(first.ID)), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 8 || events[3].Reason != domain.ReasonAutoPause || events[5].Reason != domain.ReasonExtend {
		t.Error("ordered archived history lost switch or extension events")
	}
	binaryCommand(t, binary, path, "backup")
	saved("edit", "--title", "Newer discarded title", string(second.ID))
	binaryCommand(t, binary, path, "doctor")
	if err := os.WriteFile(path, []byte("deliberate temporary database damage"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	command := exec.Command(binary, "--db", path, "list")
	command.Stdout = &out
	command.Stderr = &stderr
	var failure *exec.ExitError
	if err := command.Run(); !errors.As(err, &failure) || failure.ExitCode() != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "ERR_CORRUPT:") || !strings.Contains(stderr.String(), path) {
		t.Error("damaged database must fail visibly without stdout or reset")
	}
	binaryCommand(t, binary, path, "doctor", "--repair")
	binaryCommand(t, binary, path, "doctor")
	database, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	old, err := database.BlockByID(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	active, err := database.BlockByID(context.Background(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	target, err := database.BlockByID(context.Background(), copied.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !old.Archived || old.Status != domain.Done || active.Status != domain.Paused || active.Title != "Code" || target.CopiedFromID == nil || *target.CopiedFromID != second.ID {
		t.Error("restored snapshot must retain archived, paused and copied state while discarding newer edits")
	}
}

func TestExecutableUsageFailures(t *testing.T) {
	binary := buildExecutable(t)
	path := filepath.Join(t.TempDir(), "missing.sqlite")
	for _, args := range [][]string{{"bogus"}, {"add", "09:00"}, {"edit", "1"}, {"list", "--json"}, {"--date", "2026-02-30", "list"}} {
		var out, stderr bytes.Buffer
		command := exec.Command(binary, append([]string{"--db", path}, args...)...)
		command.Stdout = &out
		command.Stderr = &stderr
		var failure *exec.ExitError
		if err := command.Run(); !errors.As(err, &failure) || failure.ExitCode() != 2 || out.Len() != 0 || !strings.HasPrefix(stderr.String(), "ERR_USAGE:") {
			t.Errorf("usage failure %v did not stay on stderr with exit 2", args)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Error("invalid syntax created planner data")
	}
}
