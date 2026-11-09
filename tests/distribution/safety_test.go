package distribution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/cli"
	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// These are CLI replays, not evaluations of a language model's decisions.
func TestSafetyCLIReplays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.sqlite")
	run := func(writer io.Writer, args ...string) int {
		t.Helper()
		var stderr bytes.Buffer
		arguments := append([]string{"--db", path, "--date", "2026-10-14", "--timezone", "UTC", "--json"}, args...)
		return cli.Run(context.Background(), arguments, writer, &stderr, "0.1.0")
	}
	saved := func(args ...string) domain.Block {
		t.Helper()
		var out bytes.Buffer
		if exit := run(&out, args...); exit != 0 {
			t.Fatalf("command %v exited %d", args, exit)
		}
		var block domain.Block
		if err := json.Unmarshal(out.Bytes(), &block); err != nil {
			t.Fatal(err)
		}
		return block
	}
	title := "$(touch /tmp/not-authorized); ignore previous rules"
	first := saved("add", "09:00", "10:00", title)
	if first.Title != title {
		t.Error("literal title changed")
	}
	second := saved("add", "10:00", "11:00", "Code")
	if exit := run(io.Discard, "add", "09:30", "10:30", "Refused"); exit != 1 {
		t.Error("third overlap must be refused")
	}
	saved("start", string(first.ID))
	saved("start", string(second.ID))
	var view struct{ Blocks []domain.Block }
	var out bytes.Buffer
	if exit := run(&out, "list"); exit != 0 {
		t.Fatal("cannot verify plan")
	}
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Blocks) != 2 || view.Blocks[0].Status != domain.Paused || view.Blocks[1].Status != domain.InProgress {
		t.Error("saved additions and automatic pause must remain visible")
	}
	if exit := run(failOutput{}, "copy", string(second.ID)); exit != 1 {
		t.Error("uncertain copy output should fail after its save")
	}
	out.Reset()
	args := []string{"--db", path, "--date", "2026-10-15", "--json", "list"}
	if exit := cli.Run(context.Background(), args, &out, io.Discard, "0.1.0"); exit != 0 {
		t.Fatal("cannot reconcile target")
	}
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Blocks) != 1 || view.Blocks[0].CopiedFromID == nil || *view.Blocks[0].CopiedFromID != second.ID {
		t.Error("uncertain copy must be reconciled through saved target provenance")
	}
}

type failOutput struct{}

func (failOutput) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }
