package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type failingWriter struct{ cause error }

func (writer failingWriter) Write([]byte) (int, error) { return 0, writer.cause }

func TestOutputFailureAfterCommit(t *testing.T) {
	f := newCommandFixture(t)
	cause := errors.New("output closed")
	app := application{out: failingWriter{cause}, now: func() time.Time { return f.now }}
	opts := options{command: "add", args: []string{"09:00", "10:00", "Study"}, path: f.path, zone: "UTC", date: "2026-10-14"}
	if err := app.execute(context.Background(), opts); !errors.Is(err, cause) {
		t.Errorf("output cause must survive: %v", err)
	}
	if len(f.view().Blocks) != 1 {
		t.Error("output failure must not undo a committed block")
	}
	var stderr bytes.Buffer
	exit := app.run(context.Background(), []string{"--db", f.path, "add", "10:00", "11:00", "Code"}, &stderr, "test")
	if exit != 1 || !strings.HasPrefix(stderr.String(), "ERR_IO:") {
		t.Error("failed output must exit nonzero with an I/O diagnostic")
	}
	if len(f.view().Blocks) != 2 {
		t.Error("runner output failure must also retain durable data")
	}
}

func TestCommandCancellation(t *testing.T) {
	f := newCommandFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	app := application{out: &bytes.Buffer{}, now: func() time.Time { return f.now }}
	if err := app.execute(ctx, options{path: f.path}); !errors.Is(err, context.Canceled) {
		t.Errorf("lost cancellation cause: %v", err)
	}
	var stderr bytes.Buffer
	if exit := app.run(ctx, []string{"--db", f.path, "list"}, &stderr, "test"); exit != 130 || !strings.HasPrefix(stderr.String(), "ERR_CANCELLED:") {
		t.Error("cancellation must retain its distinct exit and diagnostic")
	}
}
