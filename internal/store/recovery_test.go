package store

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func TestProcessMutationRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flowd.sqlite")
	child := startStoreProcess(t, "mutations", path)
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("expected forced termination")
	}
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	block, err := db.BlockByID(context.Background(), "bl_committed")
	if err != nil {
		t.Fatal(err)
	}
	if block.Status != domain.Todo || block.ActualStart != nil {
		t.Error("uncommitted update became visible")
	}
	events, err := db.EventsByBlock(context.Background(), block.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ToStatus != domain.Todo {
		t.Error("committed creation needs its event, unfinished event must disappear")
	}
	if _, err := db.BlockByID(context.Background(), "bl_uncommitted"); !errors.Is(err, ErrBlockNotFound) {
		t.Error("uncommitted block survived termination")
	}
	db.Close()
	if err := Inspect(context.Background(), path); err != nil {
		t.Fatalf("recovered file failed full inspection: %v", err)
	}
}

func TestProcessOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flowd.sqlite")
	child := startStoreProcess(t, "hold", path)
	other, err := Open(context.Background(), path)
	if other != nil {
		_ = other.Close()
		t.Fatal("competing process acquired database")
	}
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("want lock refusal, got %v", err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("expected forced termination")
	}
	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("process exit must release ownership: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func startStoreProcess(t *testing.T, mode, path string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStoreProcess$", "-test.count=1")
	child.Env = append(os.Environ(), "FLOWD_TEST_PROCESS="+mode, "FLOWD_TEST_PATH="+path)
	var stderr bytes.Buffer
	child.Stderr = &stderr
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "ready\n" {
		_ = child.Process.Kill()
		_ = child.Wait()
		t.Fatalf("child not ready: %v, response %q, stderr %s", err, line, stderr.String())
	}
	return child
}

func TestStoreProcess(t *testing.T) {
	mode := os.Getenv("FLOWD_TEST_PROCESS")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	if mode != "hold" && mode != "mutations" {
		t.Fatal("unknown subprocess mode")
	}
	store, err := Open(context.Background(), os.Getenv("FLOWD_TEST_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if mode == "mutations" {
		ctx := context.Background()
		if err := store.CreateDay(ctx, exampleDay(t)); err != nil {
			t.Fatal(err)
		}
		block := exampleBlock(t, "bl_committed", "09:00", "10:00")
		if err := store.CreateBlock(ctx, block, exampleEvent(block, block.Status, block.CreatedAt, domain.ReasonUser)); err != nil {
			t.Fatal(err)
		}
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		block.Status = domain.InProgress
		if _, err := tx.ExecContext(ctx, "UPDATE blocks SET status=?,actual_start_utc=? WHERE id=?", block.Status, block.CreatedAt.Format(time.RFC3339Nano), block.ID); err != nil {
			t.Fatal(err)
		}
		if err := insertEvent(ctx, tx, exampleEvent(block, domain.Todo, block.CreatedAt, domain.ReasonUser)); err != nil {
			t.Fatal(err)
		}
		unfinished := exampleBlock(t, "bl_uncommitted", "11:00", "12:00")
		if err := insertBlock(ctx, tx, unfinished); err != nil {
			t.Fatal(err)
		}
		if err := insertEvent(ctx, tx, exampleEvent(unfinished, unfinished.Status, unfinished.CreatedAt, domain.ReasonUser)); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("ready")
	for {
		time.Sleep(time.Hour)
	}
}
