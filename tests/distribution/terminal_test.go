package distribution

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func installerInTerminal(t *testing.T, f *releaseFixture, invocation string) (string, int) {
	t.Helper()
	script, err := exec.LookPath("script")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("native CI requires the test-only script utility")
		}
		t.Skip("terminal checks require the test-only script utility")
	}
	statusFile := filepath.Join(f.root, "terminal-status")
	program := invocation + `; result=$?; printf '%s\n' "$result" > ` + shellLiteral(statusFile) + `; exit "$result"`
	var args []string
	switch runtime.GOOS {
	case "darwin":
		args = []string{"-q", "/dev/null", "/bin/sh", "-c", program}
	case "linux":
		args = []string{"-q", "-e", "-c", "/bin/sh -c " + shellLiteral(program), "/dev/null"}
	default:
		t.Skip("terminal checks support the native macOS/Linux installer platforms")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, script, args...)
	command.Env = append(f.environment, "SHELL=/bin/sh")
	command.Dir = f.root
	// Keep the PTY input open without injecting commands or an echoed EOF character.
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	command.Stdin = input
	output, runErr := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("terminal command timed out: %v; %s", runErr, output)
	}
	contents, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("terminal driver did not record installer status: %v; %v; %s", err, runErr, output)
	}
	status, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil {
		t.Fatal(err)
	}
	if status == 0 && runErr != nil {
		t.Fatalf("terminal wrapper failed despite installer success: %v; %s", runErr, output)
	}
	return strings.ReplaceAll(string(output), "\r\n", "\n"), status
}

func terminalEnvironment(f *releaseFixture, term, noColor string) {
	filtered := make([]string, 0, len(f.environment)+2)
	for _, entry := range f.environment {
		if !strings.HasPrefix(entry, "TERM=") && !strings.HasPrefix(entry, "NO_COLOR=") {
			filtered = append(filtered, entry)
		}
	}
	if term != "" {
		filtered = append(filtered, "TERM="+term)
	}
	if noColor != "" {
		filtered = append(filtered, "NO_COLOR="+noColor)
	}
	f.environment = filtered
}

func TestInstallerTerminalPresentation(t *testing.T) {
	for _, test := range []struct {
		name, term, noColor string
		piped, redirected   bool
		styled              bool
	}{
		{"capable terminal", "xterm-256color", "", false, false, true},
		{"piped stdin", "xterm-256color", "", true, false, true},
		{"NO_COLOR", "xterm-256color", "1", false, false, false},
		{"dumb terminal", "dumb", "", false, false, false},
		{"unset TERM", "", "", false, false, false},
		{"redirected output", "xterm-256color", "", false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newReleaseFixture(t)
			terminalEnvironment(f, test.term, test.noColor)
			invocation := "/bin/sh " + shellLiteral(filepath.Join(projectRoot(t), "install.sh")) + " --version 0.1.0"
			if test.piped {
				if err := os.Symlink("/bin/sh", filepath.Join(f.tools, "sh")); err != nil {
					t.Fatal(err)
				}
				invocation = "curl -fsSL https://github.com/raqeebshafeeque/flowd/releases/latest/download/install.sh | sh -s -- --version v0.1.0"
			}
			logFile := filepath.Join(f.root, "redirected-output")
			if test.redirected {
				invocation += " > " + shellLiteral(logFile)
			}
			output, status := installerInTerminal(t, f, invocation)
			if status != 0 {
				t.Fatalf("terminal install exit %d: %s", status, output)
			}
			if test.redirected {
				contents, err := os.ReadFile(logFile)
				if err != nil {
					t.Fatal(err)
				}
				output = string(contents)
			}
			if test.styled {
				for _, want := range []string{"\x1b[36m", "\x1b[32m", "\r\x1b[2K", "✓ Selected v0.1.0", "✓ Downloaded release", "✓ Verified checksum and package", "✓ Installed binary and skill"} {
					if !strings.Contains(output, want) {
						t.Errorf("missing terminal presentation %q", want)
					}
				}
			} else {
				if strings.ContainsAny(output, "\x1b\r") {
					t.Errorf("plain mode emitted terminal controls: %q", output)
				}
				assertOutputOrder(t, output, "[1/4]", "[2/4]", "[3/4]", "[4/4]")
			}
			if !strings.Contains(output, "Installed flowd 0.1.0!") || !strings.Contains(output, "Reload your AI harness to load the flowd skill.") {
				t.Errorf("missing readable success/next steps: %s", output)
			}
			binary := filepath.Join(f.home, ".local/bin/flowd")
			command := exec.Command(binary, "--version")
			command.Env = f.environment
			if version, err := command.CombinedOutput(); err != nil || string(version) != "flowd 0.1.0\n" {
				t.Fatalf("terminal install did not publish the real binary: %v; %s", err, version)
			}
			t.Logf("%s output:\n%s", test.name, output)
		})
	}
}

func TestInstallerTerminalPublicationFailure(t *testing.T) {
	for _, kind := range []string{"SIGTERM", "cleanup failure"} {
		t.Run(kind, func(t *testing.T) {
			f := newReleaseFixture(t)
			if output, err := f.install("--version", "0.1.0"); err != nil {
				t.Fatalf("initial installation: %v; %s", err, output)
			}
			terminalEnvironment(f, "xterm", "")
			wantStatus, diagnostic := 1, "flowd install: cannot release install lock"
			if kind == "SIGTERM" {
				wantStatus, diagnostic = 143, "prior installation restored after failed publication"
				real, err := exec.LookPath("mv")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(f.tools, "mv")); err != nil {
					t.Fatal(err)
				}
				f.script("mv", `last= previous=
for argument do previous=$last; last=$argument; done
case "$previous" in */.flowd-skill.*/new) kill -TERM "$PPID"; exit 1;; esac
exec `+shellLiteral(real)+` "$@"`)
			} else {
				real, err := exec.LookPath("rmdir")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(f.tools, "rmdir")); err != nil {
					t.Fatal(err)
				}
				f.script("rmdir", `case "$1" in */.local/bin/.flowd-install.lock) exit 77;; esac
exec `+shellLiteral(real)+` "$@"`)
			}
			invocation := "/bin/sh " + shellLiteral(filepath.Join(projectRoot(t), "install.sh")) + " --version 0.1.0"
			output, status := installerInTerminal(t, f, invocation)
			if status != wantStatus || !strings.Contains(output, diagnostic) {
				t.Fatalf("terminal failure/recovery mismatch: exit %d, want %d; %s", status, wantStatus, output)
			}
			assertNoInstallSuccess(t, output)
			if _, err := os.Stat(filepath.Join(f.home, ".local/bin/flowd")); err != nil {
				t.Error("failure lost the installed executable")
			}
			contents, err := os.ReadFile(filepath.Join(f.home, ".agents/skills/flowd/VERSION"))
			if err != nil || string(contents) != "0.1.0\n" {
				t.Errorf("failure lost the matching skill: %q; %v", contents, err)
			}
		})
	}
}

func TestInstallerTerminalFailure(t *testing.T) {
	f := newReleaseFixture(t)
	terminalEnvironment(f, "xterm", "")
	value := strings.Repeat("0", 64) + "  flowd_0.1.0_" + runtimePlatform() + ".tar.gz\n"
	if err := os.WriteFile(filepath.Join(f.assets, "v0.1.0/checksums.txt"), []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	invocation := "/bin/sh " + shellLiteral(filepath.Join(projectRoot(t), "install.sh")) + " --version 0.1.0"
	output, status := installerInTerminal(t, f, invocation)
	if status != 1 || !strings.Contains(output, "\nflowd install: release checksum mismatch") {
		t.Errorf("terminal error/status was obscured: exit %d; %q", status, output)
	}
	assertNoInstallSuccess(t, output)
	if strings.Contains(output, "✓ Verified checksum and package") {
		t.Error("failed verification received a completed tick")
	}
	if _, err := os.Stat(filepath.Join(f.home, ".local/bin/flowd")); !os.IsNotExist(err) {
		t.Error("terminal validation failure published a binary")
	}
}
