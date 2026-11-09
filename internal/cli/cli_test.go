package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoveryWithoutDatabase(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_DATA_HOME", directory)
	for _, test := range []struct {
		name string
		args []string
		exit int
		text string
	}{
		{"help", []string{"--help"}, 0, "Usage:"},
		{"help ignores storage", []string{"--db", "missing/data.sqlite", "--help"}, 0, "Usage:"},
		{"json version", []string{"--json", "--version"}, 0, `"version":"test"`},
		{"version", []string{"--version"}, 0, "flowd test"},
		{"unknown", []string{"not-a-command"}, 2, "ERR_USAGE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			exit := Run(context.Background(), test.args, &out, &stderr, "test")
			if exit != test.exit || !strings.Contains(out.String()+stderr.String(), test.text) {
				t.Errorf("got exit %d, stdout %q, stderr %q", exit, out.String(), stderr.String())
			}
		})
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Error("discovery must not create a data directory")
	}
}
