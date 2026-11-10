package distribution

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func TestReadmeInstallationExamples(t *testing.T) {
	directory := noPublishPackages(t)
	f := newReleaseFixture(t)
	useRealPackages(t, f, directory, "0.1.0")
	if err := os.Symlink("/bin/sh", filepath.Join(f.tools, "sh")); err != nil {
		t.Fatal(err)
	}
	guide, _, found := strings.Cut(projectFile(t, "README.md"), "## Development\n")
	if !found {
		t.Fatal("missing development section")
	}
	blocks := regexp.MustCompile("(?s)"+`\x60\x60\x60sh\n(.*?)\n\x60\x60\x60`).FindAllStringSubmatch(guide, -1)
	if len(blocks) < 3 {
		t.Fatal("missing install, update and use examples")
	}
	if len(strings.Split(strings.TrimSpace(blocks[0][1]), "\n")) != 1 || !strings.Contains(blocks[0][1], "| sh") {
		t.Fatal("installation should use a single piped command")
	}
	script := "set -eu\n"
	for _, block := range blocks {
		script += block[1] + "\n"
	}
	command := exec.Command("/bin/sh", "-c", script)
	// Follow the guide's PATH setup before running the usage examples.
	command.Env = append(f.environment, "PATH="+filepath.Join(f.home, ".local", "bin")+string(os.PathListSeparator)+f.tools)
	command.Dir = f.root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("README examples: %v; %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(f.root, "forbidden")); !os.IsNotExist(err) {
		t.Error("installation examples required a forbidden tool")
	}
	for _, path := range []string{
		filepath.Join(f.home, "data", "flowd", "flowd.sqlite.bak"),
		filepath.Join(f.home, "work-plan.sqlite"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("documented database/backup location: %v", err)
		}
	}
	command = exec.Command(filepath.Join(f.home, ".local", "bin", "flowd"), "--json", "list")
	command.Env = f.environment
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Blocks []domain.Block `json:"blocks"`
	}
	if err := json.Unmarshal(output, &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Blocks) != 2 {
		t.Fatalf("got %d blocks after following the guide, want 2", len(view.Blocks))
	}
	if view.Blocks[0].Title != "Study" || view.Blocks[0].Status != domain.Done {
		t.Error("study block was not completed")
	}
	if view.Blocks[1].Title != "Code" || view.Blocks[1].Status != domain.Todo {
		t.Error("code block should still be planned")
	}
}
