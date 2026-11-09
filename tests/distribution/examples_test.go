package distribution

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func buildVersion(t *testing.T, version string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "flowd")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-ldflags", "-X main.version="+version, "-o", binary, "./cmd/flowd")
	command.Dir = projectRoot(t)
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v; %s", err, output)
	}
	return binary
}

func TestSkillExamples(t *testing.T) {
	reference := projectFile(t, "skills/flowd/references/commands.md")
	match := regexp.MustCompile("(?s)" + `\x60\x60\x60json\n(.*?)\n\x60\x60\x60`).FindStringSubmatch(reference)
	if len(match) != 2 {
		t.Fatal("missing replayable JSON example")
	}
	var steps []struct {
		Args            []string
		Capture, Status string
		Events          int
	}
	if err := json.Unmarshal([]byte(match[1]), &steps); err != nil {
		t.Fatal(err)
	}
	binary := buildVersion(t, "0.1.0")
	path := filepath.Join(t.TempDir(), "sandbox.sqlite")
	ids := map[string]string{}
	for _, step := range steps {
		arguments := []string{"--db", path, "--date", "2026-10-14", "--timezone", "UTC", "--json"}
		for _, argument := range step.Args {
			if id, ok := ids[argument]; ok {
				argument = id
			}
			arguments = append(arguments, argument)
		}
		var out, stderr bytes.Buffer
		command := exec.Command(binary, arguments...)
		command.Stdout = &out
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("example %v: %v; %s", step.Args, err, stderr.String())
		}
		if step.Status != "" {
			var block domain.Block
			if err := json.Unmarshal(out.Bytes(), &block); err != nil {
				t.Fatal(err)
			}
			if string(block.Status) != step.Status {
				t.Errorf("got status %s, want %s", block.Status, step.Status)
			}
			if step.Capture != "" {
				if block.ID == "" {
					t.Fatal("missing captured id")
				}
				ids[step.Capture] = string(block.ID)
			}
		}
		if step.Events > 0 {
			var events []domain.BlockEvent
			if err := json.Unmarshal(out.Bytes(), &events); err != nil {
				t.Fatal(err)
			}
			if len(events) != step.Events {
				t.Errorf("got %d events, want %d", len(events), step.Events)
			}
		}
	}
}
