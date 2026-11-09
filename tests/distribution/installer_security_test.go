package distribution

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstallerRejectsMultilineVersionsBeforeDownload(t *testing.T) {
	for _, version := range []string{"0.1.0\njunk", "junk\n0.1.0", "0.1.0\n", "0.1.0\r", "0.1.0\n0.2.0"} {
		t.Run(version, func(t *testing.T) {
			f := newReleaseFixture(t)
			output, err := f.validate("--version", version)
			if err == nil || !strings.Contains(output, "normal X.Y.Z") {
				t.Errorf("malformed version must fail validation: %v; %s", err, output)
			}
			if _, err := os.Stat(f.requests); !os.IsNotExist(err) {
				t.Error("malformed version attempted a download")
			}
		})
	}
}

func TestReleaseStagingRejectsMultilineVersions(t *testing.T) {
	for _, version := range []string{"0.1.0\njunk", "junk\n0.1.0", "0.1.0\n", "0.1.0\r"} {
		t.Run(version, func(t *testing.T) {
			directory := t.TempDir()
			command := exec.Command("/bin/sh", filepath.Join(projectRoot(t), "scripts/stage-release.sh"), version)
			command.Dir = directory
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "normal version") {
				t.Errorf("malformed staging version must fail validation: %v; %s", err, output)
			}
			if _, err := os.Stat(filepath.Join(directory, ".release")); !os.IsNotExist(err) {
				t.Error("malformed version created release staging")
			}
		})
	}
}

func TestInstallerPreflightUsesPrivateEnvironment(t *testing.T) {
	f := newReleaseFixture(t)
	f.environment = append(f.environment, "FLOWD_REVIEW_SECRET=not-for-the-release-executable")
	f.release("0.1.0", func(entries []archiveEntry) []archiveEntry {
		entries[0].data = []byte(`#!/bin/sh
set -eu
[ -z "${FLOWD_REVIEW_SECRET:-}" ] || exit 42
[ "$PWD" = "$HOME" ] || exit 43
mkdir -p "$XDG_DATA_HOME"
printf touched > "$XDG_DATA_HOME/preflight-probe"
case "$1" in
 --version) printf 'flowd 0.1.0\n';;
 --json) printf '{"version":"0.1.0"}\n';;
 --help) printf 'Usage: flowd\n';;
 *) exit 44;;
esac
`)
		return entries
	})
	if output, err := f.validate("--version", "0.1.0"); err != nil {
		t.Fatalf("isolated preflight: %v; %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(f.home, "data")); !os.IsNotExist(err) {
		t.Error("preflight inherited the user's planner location")
	}
}

func TestInstallerBusyDestinationsAreNotOverridden(t *testing.T) {
	for _, parent := range []string{".local/bin", ".agents/skills"} {
		t.Run(parent, func(t *testing.T) {
			f := newReleaseFixture(t)
			lock := filepath.Join(f.home, parent, ".flowd-install.lock")
			if err := os.MkdirAll(lock, 0o700); err != nil {
				t.Fatal(err)
			}
			owner := filepath.Join(lock, "owner")
			if err := os.WriteFile(owner, []byte("another operation"), 0o600); err != nil {
				t.Fatal(err)
			}
			output, err := f.install("--version", "0.1.0", "--force")
			if err == nil || !strings.Contains(output, "install lock") {
				t.Errorf("busy destination must refuse even with force: %v; %s", err, output)
			}
			if got, err := os.ReadFile(owner); err != nil || string(got) != "another operation" {
				t.Error("installer removed another operation's ownership")
			}
			for _, target := range []string{".local/bin/flowd", ".agents/skills/flowd"} {
				if _, err := os.Lstat(filepath.Join(f.home, target)); !os.IsNotExist(err) {
					t.Errorf("busy destination published %s", target)
				}
			}
		})
	}
}

func TestInstallerConcurrentPublication(t *testing.T) {
	f := newReleaseFixture(t)
	if output, err := f.install("--version", "0.1.0"); err != nil {
		t.Fatalf("initial installation: %v; %s", err, output)
	}
	realMV, err := exec.LookPath("mv")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.tools, "mv")); err != nil {
		t.Fatal(err)
	}
	f.script("mv", `last= previous=
for arg do previous=$last; last=$arg; done
case "$previous" in */.flowd-bin.*/new)
 printf 'publication-ready\n'
 read -r acknowledgement
 ;;
esac
exec `+shellLiteral(realMV)+` "$@"`)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", filepath.Join(projectRoot(t), "install.sh"), "--version", "0.1.0")
	command.Env = f.environment
	command.Dir = f.root
	var stderr bytes.Buffer
	command.Stderr = &stderr
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := io.WriteString(input, "continue\n"); err != nil {
			t.Error(err)
		}
		input.Close()
		if err := command.Wait(); err != nil {
			t.Errorf("owning installer failed: %v; %s", err, &stderr)
		}
	}()
	ready, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || ready != "publication-ready\n" {
		t.Fatalf("publication handshake: %v; %q", err, ready)
	}
	second, err := f.install("--version", "0.1.0", "--force")
	if err == nil || !strings.Contains(second, "install lock") {
		t.Errorf("concurrent installer entered publication: %v; %s", err, second)
	}
}

func TestInstallerRejectsNestedDestinations(t *testing.T) {
	f := newReleaseFixture(t)
	skill := filepath.Join(f.home, ".agents/skills/flowd")
	output, err := f.install("--version", "0.1.0", "--bin-dir", filepath.Join(skill, "bin"), "--force")
	if err == nil || !strings.Contains(output, "inside the managed skill") {
		t.Errorf("nested destinations must fail before publication: %v; %s", err, output)
	}
	if matches, _ := filepath.Glob(filepath.Join(skill, "bin/.flowd-*")); len(matches) != 0 {
		t.Error("nested destinations retained publication staging")
	}
}
