package distribution

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerPublicationRollback(t *testing.T) {
	f := newReleaseFixture(t)
	if output, err := f.install("--version", "0.1.0"); err != nil {
		t.Fatalf("initial: %v; %s", err, output)
	}
	binary := filepath.Join(f.home, ".local/bin/flowd")
	skill := filepath.Join(f.home, ".agents/skills/flowd")
	before, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	oldSkill := projectFile(t, "skills/flowd/SKILL.md")
	f.binary = buildVersion(t, "0.2.0")
	f.release("0.2.0", nil)
	realMV, err := exec.LookPath("mv")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.tools, "mv")); err != nil {
		t.Fatal(err)
	}
	f.script("mv", `last= previous=
for arg do previous=$last; last=$arg; done
case "$previous" in */.flowd-skill.*/new) printf '%s\n' injected-publication-failure >&2; exit 1;; esac
exec `+shellLiteral(realMV)+` "$@"`)
	output, err := f.install("--version", "0.2.0")
	if err == nil {
		t.Fatalf("injected second-path failure succeeded: %s", output)
	}
	assertNoInstallSuccess(t, output)
	after, err := os.ReadFile(binary)
	if err != nil || !bytes.Equal(before, after) {
		t.Error("failed upgrade did not restore prior binary")
	}
	contents, err := os.ReadFile(filepath.Join(skill, "SKILL.md"))
	if err != nil || string(contents) != oldSkill {
		t.Error("failed upgrade did not restore prior skill")
	}
	version, err := os.ReadFile(filepath.Join(skill, "VERSION"))
	if err != nil || string(version) != "0.1.0\n" {
		t.Error("failed upgrade changed installed skill version")
	}
	matches, _ := filepath.Glob(filepath.Join(f.home, ".local/bin/.flowd-*"))
	if len(matches) != 0 {
		t.Error("successful rollback leaked owned publication staging")
	}
}

func TestInstallerRollbackFailure(t *testing.T) {
	f := newReleaseFixture(t)
	if output, err := f.install("--version", "0.1.0"); err != nil {
		t.Fatalf("initial: %v; %s", err, output)
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
case "$previous" in */.flowd-skill.*/new|*/.flowd-bin.*/old) exit 1;; esac
exec `+shellLiteral(realMV)+` "$@"`)
	output, err := f.install("--version", "0.1.0")
	if err == nil || !strings.Contains(output, "restoration incomplete") || !strings.Contains(output, "recovery:") {
		t.Fatalf("rollback failure must disclose retained recovery paths: %v; %s", err, output)
	}
	assertNoInstallSuccess(t, output)
	backups, _ := filepath.Glob(filepath.Join(f.home, ".local/bin/.flowd-bin.*/old"))
	if len(backups) != 1 {
		t.Error("rollback failure removed the retained prior executable")
	}
	if err := os.Remove(filepath.Join(f.tools, "mv")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realMV, filepath.Join(f.tools, "mv")); err != nil {
		t.Fatal(err)
	}
	if output, err := f.install("--version", "0.1.0"); err != nil {
		t.Fatalf("explicit rerun should restore the managed pair: %v; %s", err, output)
	}
}

func TestInstallerInterruptedPublication(t *testing.T) {
	f := newReleaseFixture(t)
	if output, err := f.install("--version", "0.1.0"); err != nil {
		t.Fatalf("initial: %v; %s", err, output)
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
case "$previous" in */.flowd-skill.*/new) kill -TERM "$PPID"; exit 1;; esac
exec `+shellLiteral(realMV)+` "$@"`)
	output, err := f.install("--version", "0.1.0")
	if err == nil {
		t.Fatalf("interrupted installation reported success: %s", output)
	}
	assertNoInstallSuccess(t, output)
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 143 {
		t.Errorf("SIGTERM status = %v, want exit 143", err)
	}
	version, err := os.ReadFile(filepath.Join(f.home, ".agents/skills/flowd/VERSION"))
	if err != nil || string(version) != "0.1.0\n" {
		t.Error("handled interrupt lost prior skill")
	}
	if _, err := os.Stat(filepath.Join(f.home, ".local/bin/flowd")); err != nil {
		t.Error("handled interrupt lost prior executable")
	}
}

func shellLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
