package distribution

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func noPublishPackages(t *testing.T) string { t.Helper(); return noPublishVersion(t, "0.1.0") }

func noPublishVersion(t *testing.T, version string) string {
	t.Helper()
	tool := os.Getenv("FLOWD_GORELEASER")
	if tool == "" {
		var err error
		tool, err = exec.LookPath("goreleaser")
		if err != nil {
			t.Skip("set FLOWD_GORELEASER for actual packaging checks")
		}
	}
	directory := t.TempDir()
	list := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	list.Dir = projectRoot(t)
	output, err := list.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range bytes.Split(output, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		path := string(name)
		source := filepath.Join(projectRoot(t), path)
		info, err := os.Lstat(source)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("unsupported source fixture %s", path)
		}
		content, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(directory, path)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, content, info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Fixture"}, {"config", "user.email", "fixture@example.invalid"}, {"add", "."}, {"commit", "-qm", "fixture"}, {"tag", "v" + version}, {"remote", "add", "origin", "https://github.com/raqeebshafeeque/flowd.git"}} {
		command := exec.Command("git", args...)
		command.Dir = directory
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v; %s", err, output)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, tool, "release", "--skip=publish", "--clean")
	command.Dir = directory
	command.Env = append(os.Environ(), "GITHUB_TOKEN=", "GITLAB_TOKEN=", "GITEA_TOKEN=")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("no-publish build: %v; %s", err, output)
	}
	return filepath.Join(directory, "dist")
}

func TestNoPublishBuild(t *testing.T) {
	directory := noPublishPackages(t)
	for _, target := range []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"} {
		if _, err := os.Stat(filepath.Join(directory, "flowd_0.1.0_"+target+".tar.gz")); err != nil {
			t.Error(err)
		}
	}
}

func archiveFiles(t *testing.T, path string) map[string][]byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	result := map[string][]byte{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			t.Errorf("unexpected archive type %s: %d", header.Name, header.Typeflag)
		}
		if _, exists := result[header.Name]; exists {
			t.Errorf("duplicate archive member %s", header.Name)
		}
		contents, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		result[header.Name] = contents
	}
	return result
}

func TestGeneratedPackageContract(t *testing.T) {
	directory := noPublishPackages(t)
	expected := []string{"LICENSE", "VERSION", "flowd", "skills/flowd/SKILL.md", "skills/flowd/VERSION", "skills/flowd/references/commands.md", "skills/flowd/references/safety.md"}
	sort.Strings(expected)
	for _, target := range []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"} {
		files := archiveFiles(t, filepath.Join(directory, "flowd_0.1.0_"+target+".tar.gz"))
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		if strings.Join(names, "\n") != strings.Join(expected, "\n") {
			t.Errorf("unexpected %s package members: %v", target, names)
		}
		for _, name := range []string{"VERSION", "skills/flowd/VERSION"} {
			if string(files[name]) != "0.1.0\n" {
				t.Errorf("wrong %s version record", target)
			}
		}
		if string(files["skills/flowd/SKILL.md"]) != projectFile(t, "skills/flowd/SKILL.md") {
			t.Errorf("%s skill does not match source", target)
		}
	}
}

func TestGeneratedChecksumsAndInstall(t *testing.T) {
	directory := noPublishPackages(t)
	checksums, err := os.ReadFile(filepath.Join(directory, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{"install.sh": true}
	for _, target := range []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"} {
		expected["flowd_0.1.0_"+target+".tar.gz"] = true
	}
	for _, line := range strings.Split(strings.TrimSpace(string(checksums)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatal("malformed generated checksum")
		}
		name := fields[1]
		if !expected[name] {
			t.Errorf("unexpected generated checksum %s", name)
		}
		delete(expected, name)
		path := filepath.Join(directory, name)
		if name == "install.sh" {
			path = filepath.Join(filepath.Dir(directory), "install.sh")
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(contents)
		if hex.EncodeToString(digest[:]) != fields[0] {
			t.Errorf("wrong digest for %s", name)
		}
	}
	if len(expected) != 0 {
		t.Errorf("missing generated checksums: %v", expected)
	}
	f := newReleaseFixture(t)
	release := filepath.Join(f.assets, "v0.1.0")
	for _, name := range []string{"checksums.txt", "flowd_0.1.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"} {
		contents, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(release, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	output, err := f.install("--version", "0.1.0")
	if err != nil {
		t.Fatalf("real package install: %v; %s", err, output)
	}
	installed := filepath.Join(f.home, ".local/bin/flowd")
	for _, args := range [][]string{{"--version"}, {"--help"}, {"--db", filepath.Join(f.root, "sandbox.sqlite"), "--date", "2026-10-14", "--timezone", "UTC", "--json", "add", "09:00", "10:00", "Sandbox"}} {
		command := exec.Command(installed, args...)
		command.Dir = f.home
		command.Env = f.environment
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("installed package command: %v; %s", err, output)
		}
	}
}

func TestReleasePRMetadataAccepted(t *testing.T) {
	directory := noPublishPackages(t)
	checkout := filepath.Dir(directory)
	if err := os.WriteFile(filepath.Join(checkout, ".release-please-manifest.json"), []byte("{\".\":\"0.1.0\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "test", "./tests/distribution", "-run", "^TestReleasePolicy$", "-count=1")
	command.Dir = checkout
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated release PR metadata must pass CI: %v; %s", err, output)
	}
}

func TestReleaseConfiguration(t *testing.T) {
	config := projectFile(t, ".goreleaser.yaml")
	for _, contract := range []string{"CGO_ENABLED=0", "darwin", "linux", "amd64", "arm64", "main.version=", "checksums.txt", "install.sh", "use_existing_draft: true", "draft: true"} {
		if !strings.Contains(config, contract) {
			t.Errorf("missing release contract %q", contract)
		}
	}
}
