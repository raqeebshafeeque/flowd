package distribution

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

func TestReadmeInstallationExamples(t *testing.T) {
	directory := noPublishPackages(t)
	f := newReleaseFixture(t)
	useRealPackages(t, f, directory, "0.1.0")
	if err := os.Symlink("/bin/sh", filepath.Join(f.tools, "sh")); err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)"+`\x60\x60\x60sh\n(.*?)\n\x60\x60\x60`).FindAllStringSubmatch(projectFile(t, "README.md"), -1)
	if len(blocks) < 3 {
		t.Fatal("missing install, update and use examples")
	}
	script := "set -eu\n" + blocks[0][1] + "\n" + blocks[1][1] + "\n" + blocks[2][1] + "\n"
	command := exec.Command("/bin/sh", "-c", script)
	command.Env = f.environment
	command.Dir = f.root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("README examples: %v; %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(f.root, "forbidden")); !os.IsNotExist(err) {
		t.Error("installation examples required a forbidden tool")
	}
}
