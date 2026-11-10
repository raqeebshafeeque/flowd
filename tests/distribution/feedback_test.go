package distribution

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInstallerFeedback(t *testing.T) {
	for _, test := range []struct {
		name, version, directory string
		inPath                   bool
	}{
		{"latest default", "", ".local/bin", false},
		{"pinned version", "0.2.0", ".local/bin", false},
		{"literal custom path", "0.1.0", "custom bin '$(touch nope)", false},
		{"already on PATH", "0.1.0", ".local/bin", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newReleaseFixture(t)
			version := "0.1.0"
			if test.version == "0.2.0" {
				version = test.version
				f.binary = buildVersion(t, version)
				f.release(version, nil)
			}
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
			args := []string{filepath.Join(projectRoot(t), "install.sh"), "--bin-dir", bin}
			if test.version != "" {
				args = append(args, "--version", "v"+test.version)
			}
			command := exec.Command("/bin/sh", args...)
			command.Env, command.Dir = f.environment, f.root
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("installation: %v; %s", err, &stderr)
			}
			platform := "Linux"
			if runtime.GOOS == "darwin" {
				platform = "macOS"
			}
			output := stdout.String()
			assertOutputOrder(t, output,
				"[1/4] Selecting release...",
				"[2/4] Downloading flowd "+version+" for "+platform+" "+runtime.GOARCH+"...",
				"[3/4] Verifying checksum and package...",
				"[4/4] Installing binary and skill...",
				"Installed flowd "+version+"!",
				"  Binary  "+filepath.Join(bin, "flowd"),
				"  Skill   ",
				"  PATH    ",
				"  Next    Reload your AI harness to load the flowd skill.",
			)
			if test.inPath {
				if !strings.Contains(output, "Already on PATH: "+bin) || strings.Contains(output, "Add "+bin) {
					t.Errorf("PATH readiness must be explicit without redundant setup: %s", output)
				}
			} else if !strings.Contains(output, "Add "+bin+" to your shell's PATH.") {
				t.Errorf("missing selected-directory guidance: %s", output)
			}
			for _, forbidden := range []string{"\x1b", "\r", "Pi:", "/reload", "/skill:flowd"} {
				if strings.Contains(output, forbidden) {
					t.Errorf("captured feedback contains terminal controls or harness-specific advice: %q", forbidden)
				}
			}
			if stderr.Len() != 0 {
				t.Errorf("successful install wrote diagnostics: %s", &stderr)
			}
			if _, err := os.Stat(filepath.Join(f.root, "nope")); !os.IsNotExist(err) {
				t.Error("custom path executed shell text")
			}
			if _, err := os.Stat(filepath.Join(f.home, "data")); !os.IsNotExist(err) {
				t.Error("feedback installation touched planner data")
			}
		})
	}
}

func TestInstallerFeedbackWithoutPublication(t *testing.T) {
	f := newReleaseFixture(t)
	for _, test := range []struct {
		name string
		args []string
	}{
		{"help", []string{"--help"}},
		{"invalid option", []string{"--not-a-flag"}},
		{"invalid version", []string{"--version", "not-a-version"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("/bin/sh", append([]string{filepath.Join(projectRoot(t), "install.sh")}, test.args...)...)
			command.Env, command.Dir = f.environment, f.root
			var stdout bytes.Buffer
			command.Stdout = &stdout
			err := command.Run()
			if (err == nil) != (test.name == "help") {
				t.Fatalf("unexpected argument status: %v; %s", err, &stdout)
			}
			assertNoInstallSuccess(t, stdout.String())
			if strings.Contains(stdout.String(), "[1/4]") {
				t.Error("help/invalid arguments printed install progress")
			}
		})
	}
	output, err := f.validate("--version", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	assertNoInstallSuccess(t, output)
	if strings.Contains(output, "[4/4]") {
		t.Error("prepare-only validation announced file installation")
	}
	if _, err := os.Stat(filepath.Join(f.home, ".local/bin/flowd")); !os.IsNotExist(err) {
		t.Error("prepare-only validation installed an executable")
	}
}

func TestInstallerFeedbackFailures(t *testing.T) {
	for _, test := range []struct {
		name, version, diagnostic string
		prepare                   func(*releaseFixture)
	}{
		{"unavailable version", "0.9.0", "flowd install: release checksums unavailable", nil},
		{"unavailable archive", "0.1.0", "flowd install: release archive unavailable", func(f *releaseFixture) {
			if err := os.Remove(filepath.Join(f.assets, "v0.1.0", "flowd_0.1.0_"+runtimePlatform()+".tar.gz")); err != nil {
				f.t.Fatal(err)
			}
		}},
		{"checksum mismatch", "0.1.0", "flowd install: release checksum mismatch", func(f *releaseFixture) {
			value := strings.Repeat("0", 64) + "  flowd_0.1.0_" + runtimePlatform() + ".tar.gz\n"
			if err := os.WriteFile(filepath.Join(f.assets, "v0.1.0/checksums.txt"), []byte(value), 0o600); err != nil {
				f.t.Fatal(err)
			}
		}},
		{"unexpected archive member", "0.1.0", "flowd install: unexpected, duplicate or missing archive members", func(f *releaseFixture) {
			f.release("0.1.0", func(entries []archiveEntry) []archiveEntry {
				return append(entries, archiveEntry{name: "extra", data: []byte("unexpected")})
			})
		}},
		{"unmanaged target", "0.1.0", "flowd install: unmanaged flowd destination exists", func(f *releaseFixture) {
			bin := filepath.Join(f.home, ".local/bin")
			if err := os.MkdirAll(bin, 0o700); err != nil {
				f.t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, "flowd"), []byte("unmanaged"), 0o700); err != nil {
				f.t.Fatal(err)
			}
		}},
		{"staging cleanup", "0.1.0", "injected staging cleanup failure", func(f *releaseFixture) {
			real, err := exec.LookPath("rm")
			if err != nil {
				f.t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(f.tools, "rm")); err != nil {
				f.t.Fatal(err)
			}
			f.script("rm", `for argument do
 case "$argument" in */.flowd-bin.*) printf 'injected staging cleanup failure\n' >&2; exit 77;; esac
done
exec `+shellLiteral(real)+` "$@"`)
		}},
		{"lock cleanup", "0.1.0", "flowd install: cannot release install lock", func(f *releaseFixture) {
			real, err := exec.LookPath("rmdir")
			if err != nil {
				f.t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(f.tools, "rmdir")); err != nil {
				f.t.Fatal(err)
			}
			f.script("rmdir", `case "$1" in */.local/bin/.flowd-install.lock) exit 77;; esac
exec `+shellLiteral(real)+` "$@"`)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newReleaseFixture(t)
			if test.prepare != nil {
				test.prepare(f)
			}
			sentinels := []string{"data/flowd.sqlite", "data/flowd.sqlite.bak", ".zshrc", ".bashrc", ".config/fish/config.fish"}
			for _, path := range sentinels {
				path = filepath.Join(f.home, path)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("unchanged"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			command := exec.Command("/bin/sh", filepath.Join(projectRoot(t), "install.sh"), "--version", test.version)
			command.Env, command.Dir = f.environment, f.root
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			err := command.Run()
			if err == nil || !strings.Contains(stderr.String(), test.diagnostic) {
				t.Fatalf("missing failure/diagnostic: %v; %s", err, &stderr)
			}
			assertNoInstallSuccess(t, stdout.String())
			for _, path := range sentinels {
				contents, err := os.ReadFile(filepath.Join(f.home, path))
				if err != nil || string(contents) != "unchanged" {
					t.Errorf("feedback failure changed %s: %q; %v", path, contents, err)
				}
			}
		})
	}
}

func assertNoInstallSuccess(t *testing.T, output string) {
	t.Helper()
	for _, forbidden := range []string{"Installed flowd", "Installed binary and skill"} {
		if strings.Contains(output, forbidden) {
			t.Errorf("failed/incomplete operation reported success %q: %s", forbidden, output)
		}
	}
}

func assertOutputOrder(t *testing.T, output string, fragments ...string) {
	t.Helper()
	remaining := output
	for _, fragment := range fragments {
		_, rest, ok := strings.Cut(remaining, fragment)
		if !ok {
			t.Errorf("missing or out-of-order output %q in:\n%s", fragment, output)
			return
		}
		remaining = rest
	}
}

func TestInstallerProgressBeforeBlockedDownload(t *testing.T) {
	f := newReleaseFixture(t)
	original := filepath.Join(f.tools, "curl-download")
	if err := os.Rename(filepath.Join(f.tools, "curl"), original); err != nil {
		t.Fatal(err)
	}
	f.script("curl", `for argument do
 case "$argument" in https://*.tar.gz)
  printf 'download-ready\n'
  read -r acknowledgement
  ;;
 esac
done
exec `+shellLiteral(original)+` "$@"`)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", filepath.Join(projectRoot(t), "install.sh"), "--version", "0.1.0")
	command.Env, command.Dir = f.environment, f.root
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(output)
	defer func() {
		io.WriteString(input, "continue\n")
		input.Close()
		io.Copy(io.Discard, reader)
		if err := command.Wait(); err != nil {
			t.Errorf("released installer failed: %v; %s", err, &stderr)
		}
	}()
	var before strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("download handshake: %v; %s", err, before.String())
		}
		if line == "download-ready\n" {
			break
		}
		before.WriteString(line)
	}
	if !strings.Contains(before.String(), "[2/4] Downloading flowd") {
		t.Errorf("blocking download began before visible progress: %q", before.String())
	}
	for _, forbidden := range []string{"[3/4]", "[4/4]", "Installed flowd"} {
		if strings.Contains(before.String(), forbidden) {
			t.Errorf("unfinished download reported later completion %q", forbidden)
		}
	}
}
