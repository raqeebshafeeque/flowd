package distribution

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func noPublishPackages(t *testing.T) string { t.Helper(); return noPublishVersion(t, "0.1.0") }

func noPublishVersion(t *testing.T, version string) string {
	t.Helper()
	tool, err := packageTool()
	if errors.Is(err, errOptionalPackageTool) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	source := projectRoot(t)
	build := func(ctx context.Context, directory, version string) error {
		return buildPackageFixture(ctx, source, tool, directory, version)
	}
	checkout, err := suitePackages.checkout(version, build, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(checkout, "dist")
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
	expected := []string{"LICENSE", "THIRD_PARTY_NOTICES", "VERSION", "flowd", "skills/flowd/SKILL.md", "skills/flowd/VERSION", "skills/flowd/references/commands.md", "skills/flowd/references/safety.md"}
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
		for _, notice := range []string{"LICENSE", "THIRD_PARTY_NOTICES"} {
			if string(files[notice]) != projectFile(t, notice) {
				t.Errorf("%s does not preserve %s", target, notice)
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
