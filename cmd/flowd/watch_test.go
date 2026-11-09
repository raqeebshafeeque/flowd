package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"github.com/raqeebshafeeque/flowd/internal/store"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExecutableWatchInterrupt(t *testing.T) {
	binary := buildExecutable(t)
	path := filepath.Join(t.TempDir(), "watch.sqlite")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, binary, "--db", path, "--date", "2026-10-14", "--timezone", "UTC", "--json", "watch")
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
			child.Process.Kill()
			child.Wait()
		}
	})
	line, err := bufio.NewReader(stdout).ReadBytes('\n')
	if err != nil || !json.Valid(line) {
		t.Fatalf("watch handshake failed: %v; %q", err, line)
	}
	if err := child.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil {
		t.Fatalf("interrupt should exit cleanly: %v; %s", err, stderr.String())
	}
	database, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("watch retained ownership: %v", err)
	}
	database.Close()
}
