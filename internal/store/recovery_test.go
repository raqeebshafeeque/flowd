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
)

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
	if mode != "hold" {
		t.Fatal("unknown subprocess mode")
	}
	store, err := Open(context.Background(), os.Getenv("FLOWD_TEST_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fmt.Println("ready")
	for {
		time.Sleep(time.Hour)
	}
}
