package distribution

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerPipedBootstrap(t *testing.T) {
	for _, test := range []struct{ name, version, arguments string }{
		{"latest", "0.1.0", ""},
		{"pinned", "0.2.0", " -s -- --version v0.2.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newReleaseFixture(t)
			if test.version != "0.1.0" {
				f.binary = buildVersion(t, test.version)
				f.release(test.version, nil)
			}
			if err := os.Symlink("/bin/sh", filepath.Join(f.tools, "sh")); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("/bin/sh", "-c", "curl -fsSL https://github.com/raqeebshafeeque/flowd/releases/latest/download/install.sh | sh"+test.arguments)
			command.Env = f.environment
			command.Dir = f.root
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("piped installation: %v; %s", err, output)
			}
			// A plain shell pipeline can return zero when curl fails and sh receives no script.
			// Check the resulting files and versions rather than trusting that exit code alone.
			binary := filepath.Join(f.home, ".local", "bin", "flowd")
			installed := exec.Command(binary, "--version")
			installed.Env = f.environment
			installed.Dir = f.root
			version, err := installed.CombinedOutput()
			if err != nil || string(version) != "flowd "+test.version+"\n" {
				t.Fatalf("installed executable version: %v; %s; bootstrap: %s", err, version, output)
			}
			skillVersion, err := os.ReadFile(filepath.Join(f.home, ".agents", "skills", "flowd", "VERSION"))
			if err != nil || string(skillVersion) != test.version+"\n" {
				t.Errorf("installed skill version: %v; %q", err, skillVersion)
			}
			if _, err := os.Stat(filepath.Join(f.home, "data")); !os.IsNotExist(err) {
				t.Error("piped installation touched planner data")
			}
			if _, err := os.Stat(filepath.Join(f.root, "forbidden")); !os.IsNotExist(err) {
				t.Error("piped installation invoked a build tool")
			}
		})
	}
}

func TestInstallerShellPathHint(t *testing.T) {
	for _, test := range []struct {
		name, directory string
		inPath          bool
	}{
		{"default directory missing", ".local/bin", false},
		{"custom directory missing", "custom bin", false},
		{"directory already present", ".local/bin", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newReleaseFixture(t)
			bin := filepath.Join(f.home, test.directory)
			if err := os.MkdirAll(bin, 0o700); err != nil {
				t.Fatal(err)
			}
			bin, err := filepath.EvalSymlinks(bin)
			if err != nil {
				t.Fatal(err)
			}
			if test.inPath {
				f.environment = append(f.environment, "PATH="+bin+string(os.PathListSeparator)+f.tools)
			}
			for _, name := range []string{".zshrc", ".bashrc"} {
				if err := os.WriteFile(filepath.Join(f.home, name), []byte("original shell settings\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			command := exec.Command("/bin/sh", filepath.Join(projectRoot(t), "install.sh"), "--version", "0.1.0", "--bin-dir", bin)
			command.Env = f.environment
			command.Dir = f.root
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("installation: %v; %s", err, &stderr)
			}
			hint := "Add " + bin + " to your shell's PATH.\n"
			if test.inPath {
				if strings.Contains(stdout.String(), "to your shell's PATH") {
					t.Error("installer printed an unnecessary PATH reminder")
				}
			} else if !strings.Contains(stdout.String(), hint) {
				t.Errorf("stdout is missing the shell PATH reminder: %s", &stdout)
			}
			if strings.Contains(stderr.String(), "PATH") {
				t.Error("successful PATH guidance went to stderr")
			}
			for _, name := range []string{".zshrc", ".bashrc"} {
				contents, err := os.ReadFile(filepath.Join(f.home, name))
				if err != nil || string(contents) != "original shell settings\n" {
					t.Errorf("installer changed %s", name)
				}
			}
		})
	}
}
