package distribution

import (
	"archive/tar"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallerDiscoveryAndArguments(t *testing.T) {
	f := newReleaseFixture(t)
	output, err := f.install("--help")
	if err != nil || !strings.Contains(output, "--version") {
		t.Fatalf("help: %v; %s", err, output)
	}
	if _, err := os.Stat(f.requests); !os.IsNotExist(err) {
		t.Error("help attempted a download")
	}
	for _, args := range [][]string{{"--bogus"}, {"--version"}, {"--version", ""}, {"--version", "0.1.0", "--version", "0.2.0"}, {"trailing"}} {
		if output, err := f.install(args...); err == nil {
			t.Errorf("invalid arguments %q succeeded: %s", args, output)
		}
	}
	if _, err := os.Stat(filepath.Join(f.home, ".local")); !os.IsNotExist(err) {
		t.Error("invalid syntax created install paths")
	}
}

func TestInstallerVersionSelection(t *testing.T) {
	f := newReleaseFixture(t)
	for _, args := range [][]string{nil, {"--version", "0.1.0"}, {"--version", "v0.1.0"}} {
		output, err := f.validate(args...)
		if err != nil {
			t.Fatalf("valid selection %v: %v; %s", args, err, output)
		}
	}
	requests, err := os.ReadFile(f.requests)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(requests), "/releases/tag/") && !strings.Contains(string(requests), "/releases/latest") {
		t.Error("default selection must resolve the normal latest release")
	}
	for _, version := range []string{"main", "0.1.0-rc.1", "../v0.1.0", "0.1.0;touch nope", "01.1.0", "0.9.0"} {
		if output, err := f.validate("--version", version); err == nil {
			t.Errorf("invalid/unavailable version %q succeeded: %s", version, output)
		}
	}
	for _, location := range []string{"https://evil.example/releases/tag/v0.1.0", "https://github.com/raqeebshafeeque/flowd/releases/tag/v0.2.0-rc.1", "https://github.com/raqeebshafeeque/flowd/releases/tag/v0.2.0?draft=1"} {
		f.environment = append(f.environment, "FLOWD_TEST_LATEST="+location)
		if _, err := f.validate(); err == nil {
			t.Errorf("unsafe latest result %q succeeded", location)
		}
	}
}

func TestInstallerPlatforms(t *testing.T) {
	f := newReleaseFixture(t)
	for _, test := range []struct{ os, arch, target string }{
		{"Darwin", "arm64", "darwin_arm64"}, {"Darwin", "x86_64", "darwin_amd64"}, {"Linux", "amd64", "linux_amd64"}, {"Linux", "aarch64", "linux_arm64"}, {"Linux", "arm64", "linux_arm64"},
	} {
		t.Run(test.os+" "+test.arch, func(t *testing.T) {
			f.environment = append(f.environment, "FLOWD_TEST_OS="+test.os, "FLOWD_TEST_ARCH="+test.arch)
			output, err := f.validate("--version", "0.1.0")
			if err != nil {
				t.Fatalf("platform: %v; %s", err, output)
			}
			requests, err := os.ReadFile(f.requests)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(requests), "flowd_0.1.0_"+test.target+".tar.gz") {
				t.Error("wrong platform archive")
			}
		})
	}
	for _, test := range []struct{ os, arch string }{{"Windows", "amd64"}, {"Linux", "riscv64"}, {"Darwin", "armv7"}} {
		f.environment = append(f.environment, "FLOWD_TEST_OS="+test.os, "FLOWD_TEST_ARCH="+test.arch)
		if err := os.Remove(f.requests); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if output, err := f.validate("--version", "0.1.0"); err == nil {
			t.Errorf("unsupported platform succeeded: %s", output)
		}
		if _, err := os.Stat(f.requests); !os.IsNotExist(err) {
			t.Error("unsupported platform attempted a download")
		}
	}
}

func TestInstallerChecksums(t *testing.T) {
	f := newReleaseFixture(t)
	checksum := filepath.Join(f.assets, "v0.1.0", "checksums.txt")
	original, err := os.ReadFile(checksum)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		value []byte
	}{
		{"missing", nil}, {"duplicate", append(append([]byte{}, original...), original...)}, {"malformed", []byte("not-a-digest  flowd_0.1.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz\n")},
		{"mismatch", []byte(strings.Repeat("0", 64) + "  flowd_0.1.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz\n")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(checksum, test.value, 0o600); err != nil {
				t.Fatal(err)
			}
			if output, err := f.validate("--version", "0.1.0"); err == nil {
				t.Errorf("bad checksum accepted: %s", output)
			}
		})
	}
	if err := os.WriteFile(checksum, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.tools, "sha256sum")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if output, err := f.validate("--version", "0.1.0"); err != nil {
		t.Fatalf("shasum fallback: %v; %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(f.root, "forbidden")); !os.IsNotExist(err) {
		t.Error("installer invoked a forbidden tool")
	}
}

func TestInstallerArchiveValidation(t *testing.T) {
	f := newReleaseFixture(t)
	for _, kind := range []string{"traversal", "absolute", "symlink", "hardlink", "duplicate", "unexpected", "missing reference", "wrong record", "wrong binary"} {
		t.Run(kind, func(t *testing.T) {
			version := "0.1.0"
			if kind == "wrong binary" {
				version = "0.2.0"
			}
			f.release(version, func(entries []archiveEntry) []archiveEntry {
				switch kind {
				case "traversal":
					entries[1].name = "../escaped"
				case "absolute":
					entries[1].name = filepath.Join(f.root, "escaped")
				case "symlink":
					entries[0].kind = tar.TypeSymlink
					entries[0].link = "/bin/sh"
				case "hardlink":
					entries[0].kind = tar.TypeLink
					entries[0].link = "VERSION"
				case "duplicate":
					entries = append(entries, entries[1])
				case "unexpected":
					entries = append(entries, archiveEntry{name: "unrelated", data: []byte("extra")})
				case "missing reference":
					entries = entries[:len(entries)-1]
				case "wrong record":
					entries[2].data = []byte("9.9.9\n")
				}
				return entries
			})
			if output, err := f.validate("--version", version); err == nil {
				t.Errorf("bad archive accepted: %s", output)
			}
			if _, err := os.Stat(filepath.Join(f.root, "escaped")); !os.IsNotExist(err) {
				t.Error("archive wrote outside staging")
			}
		})
	}
}

func TestInstallerDestinations(t *testing.T) {
	f := newReleaseFixture(t)
	bin := filepath.Join(f.home, "custom bin; literal")
	skills := filepath.Join(f.home, "custom skills")
	output, err := f.install("--version", "v0.1.0", "--bin-dir", bin, "--skills-dir", skills)
	if err != nil {
		t.Fatalf("install: %v; %s", err, output)
	}
	installed := filepath.Join(bin, "flowd")
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Error("binary is not executable")
	}
	for _, name := range []string{"SKILL.md", "VERSION", "references/commands.md", "references/safety.md"} {
		if _, err := os.Stat(filepath.Join(skills, "flowd", name)); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(output, installed) || !strings.Contains(output, "0.1.0") || !strings.Contains(output, "PATH") {
		t.Error("install output must identify version, paths and PATH advice")
	}
	if _, err := os.Stat(filepath.Join(f.home, "data")); !os.IsNotExist(err) {
		t.Error("installation touched planner data")
	}
	f2 := newReleaseFixture(t)
	if output, err := f2.install("--version", "0.1.0"); err != nil {
		t.Fatalf("default install: %v; %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(f2.home, ".local/bin/flowd")); err != nil {
		t.Fatal(err)
	}
}

func TestInstallerUpgrade(t *testing.T) {
	f := newReleaseFixture(t)
	if output, err := f.install("--version", "0.1.0"); err != nil {
		t.Fatalf("initial install: %v; %s", err, output)
	}
	skill := filepath.Join(f.home, ".agents/skills/flowd")
	obsolete := filepath.Join(skill, "obsolete.md")
	if err := os.WriteFile(obsolete, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(f.home, ".agents/skills/other")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(other, "SKILL.md")
	if err := os.WriteFile(sentinel, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(f.home, "data", "flowd")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(data, "flowd.sqlite")
	if err := os.WriteFile(database, []byte("untouched planner"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.binary = buildVersion(t, "0.2.0")
	f.release("0.2.0", nil)
	if output, err := f.install("--version", "0.2.0"); err != nil {
		t.Fatalf("upgrade: %v; %s", err, output)
	}
	if _, err := os.Stat(obsolete); !os.IsNotExist(err) {
		t.Error("upgrade retained obsolete skill files")
	}
	for path, want := range map[string]string{filepath.Join(skill, "VERSION"): "0.2.0\n", sentinel: "unrelated", database: "untouched planner"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("unexpected bytes at %s: %v", path, err)
		}
	}
	record, err := os.ReadFile(filepath.Join(skill, ".flowd-install"))
	if err != nil || !strings.Contains(string(record), "0.2.0") {
		t.Error("managed upgrade needs a matching ownership record")
	}
	if output, err := f.install("--version", "0.2.0"); err != nil {
		t.Fatalf("reinstall: %v; %s", err, output)
	}
}

func TestInstallerConflicts(t *testing.T) {
	for _, kind := range []string{"unmanaged binary", "unmanaged skill", "binary symlink", "skill symlink", "binary directory"} {
		t.Run(kind, func(t *testing.T) {
			f := newReleaseFixture(t)
			bin := filepath.Join(f.home, ".local/bin")
			skills := filepath.Join(f.home, ".agents/skills")
			if err := os.MkdirAll(bin, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(skills, 0o700); err != nil {
				t.Fatal(err)
			}
			victim := filepath.Join(f.root, "unrelated")
			if err := os.WriteFile(victim, []byte("retained"), 0o600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "unmanaged binary":
				if err := os.WriteFile(filepath.Join(bin, "flowd"), []byte("custom"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "unmanaged skill":
				if err := os.Mkdir(filepath.Join(skills, "flowd"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "binary symlink":
				if err := os.Symlink(victim, filepath.Join(bin, "flowd")); err != nil {
					t.Fatal(err)
				}
			case "skill symlink":
				if err := os.Symlink(f.root, filepath.Join(skills, "flowd")); err != nil {
					t.Fatal(err)
				}
			case "binary directory":
				if err := os.Mkdir(filepath.Join(bin, "flowd"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if output, err := f.install("--version", "0.1.0"); err == nil {
				t.Errorf("collision succeeded without force: %s", output)
			}
			output, err := f.install("--version", "0.1.0", "--force")
			if strings.HasPrefix(kind, "unmanaged") {
				if err != nil {
					t.Fatalf("explicit force: %v; %s", err, output)
				}
			} else if err == nil {
				t.Error("force bypassed unsafe destination refusal")
			}
			kept, err := os.ReadFile(victim)
			if err != nil || string(kept) != "retained" {
				t.Error("installer modified a symlink target")
			}
		})
	}
}

func TestInstallerPrerequisite(t *testing.T) {
	f := newReleaseFixture(t)
	if err := os.Remove(filepath.Join(f.tools, "tar")); err != nil {
		t.Fatal(err)
	}
	output, err := f.validate("--version", "v0.1.0")
	if err == nil || !strings.Contains(output, "tar") {
		t.Fatalf("missing prerequisite: %v; %s", err, output)
	}
	if _, err := os.Stat(f.requests); !os.IsNotExist(err) {
		t.Error("missing prerequisite attempted download")
	}
}
