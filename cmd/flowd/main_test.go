package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutableDiscovery(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "flowd")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v; %s", err, output)
	}
	output, err := exec.Command(binary, "--db", filepath.Join(t.TempDir(), "day.sqlite"), "--date", "2026-10-14", "--timezone", "UTC", "--json", "list").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `"date":"2026-10-14"`) {
		t.Errorf("built read command failed: %v; %s", err, output)
	}
	for _, flag := range []string{"--help", "--version"} {
		output, err := exec.Command(binary, flag).CombinedOutput()
		if err != nil || !strings.Contains(string(output), "flowd") {
			t.Errorf("discovery failed: %v; %s", err, output)
		}
	}
}
