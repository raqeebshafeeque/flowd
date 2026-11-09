package distribution

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type releaseFixture struct {
	t                                           *testing.T
	root, home, assets, tools, requests, binary string
	environment                                 []string
}

type archiveEntry struct {
	name string
	data []byte
	kind byte
	link string
}

func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()
	root := t.TempDir()
	f := &releaseFixture{t: t, root: root, home: filepath.Join(root, "home"), assets: filepath.Join(root, "releases"), tools: filepath.Join(root, "tools"), requests: filepath.Join(root, "requests")}
	f.binary = buildVersion(t, "0.1.0")
	for _, dir := range []string{f.home, f.assets, f.tools} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, tool := range []string{"awk", "cat", "chmod", "cmp", "cp", "dirname", "grep", "mkdir", "mktemp", "mv", "rm", "sort", "tar", "sha256sum", "shasum"} {
		real, err := exec.LookPath(tool)
		if err != nil {
			continue
		}
		if err := os.Symlink(real, filepath.Join(f.tools, tool)); err != nil {
			t.Fatal(err)
		}
	}
	f.script("uname", `case "$1" in -s) printf '%s\n' "$FLOWD_TEST_OS";; -m) printf '%s\n' "$FLOWD_TEST_ARCH";; *) exit 1;; esac`)
	f.script("curl", `output= url= writeout=
while [ "$#" -gt 0 ]; do
 case "$1" in
  -o|--output) output=$2; shift 2;;
  -w|--write-out) writeout=$2; shift 2;;
  --proto|--proto-redir|--retry|--connect-timeout|--max-time|--tlsv1.2) case "$1" in --tlsv1.2) shift;; *) shift 2;; esac;;
  https://*) url=$1; shift;;
  -*) shift;;
  *) exit 99;;
 esac
done
printf '%s\n' "$url" >> "$FLOWD_TEST_REQUESTS"
if [ "$url" = 'https://github.com/raqeebshafeeque/flowd/releases/latest' ]; then
 printf '%s' "$FLOWD_TEST_LATEST"; exit 0
fi
case "$url" in https://github.com/raqeebshafeeque/flowd/releases/download/*) ;; *) exit 22;; esac
relative=${url#https://github.com/raqeebshafeeque/flowd/releases/download/}
[ -f "$FLOWD_TEST_RELEASES/$relative" ] || exit 22
[ -n "$output" ] || exit 99
cp "$FLOWD_TEST_RELEASES/$relative" "$output"`)
	for _, tool := range []string{"go", "git", "gh", "python", "python3", "jq", "sudo"} {
		f.script(tool, `printf '%s\n' forbidden >> "$FLOWD_TEST_FORBIDDEN"; exit 99`)
	}
	osName := "Linux"
	if runtime.GOOS == "darwin" {
		osName = "Darwin"
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	f.environment = append(os.Environ(), "HOME="+f.home, "XDG_DATA_HOME="+filepath.Join(f.home, "data"), "PATH="+f.tools, "TMPDIR="+root, "FLOWD_TEST_OS="+osName, "FLOWD_TEST_ARCH="+arch, "FLOWD_TEST_RELEASES="+f.assets, "FLOWD_TEST_REQUESTS="+f.requests, "FLOWD_TEST_FORBIDDEN="+filepath.Join(root, "forbidden"), "FLOWD_TEST_LATEST=https://github.com/raqeebshafeeque/flowd/releases/tag/v0.1.0")
	f.release("0.1.0", nil)
	return f
}

func (f *releaseFixture) script(name, body string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.tools, name), []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0o700); err != nil {
		f.t.Fatal(err)
	}
}

func (f *releaseFixture) entries(version string) []archiveEntry {
	f.t.Helper()
	binary, err := os.ReadFile(f.binary)
	if err != nil {
		f.t.Fatal(err)
	}
	entries := []archiveEntry{{name: "flowd", data: binary}, {name: "VERSION", data: []byte(version + "\n")}, {name: "skills/flowd/VERSION", data: []byte(version + "\n")}, {name: "LICENSE", data: []byte(projectFile(f.t, "LICENSE"))}}
	for _, name := range []string{"SKILL.md", "references/commands.md", "references/safety.md"} {
		entries = append(entries, archiveEntry{name: "skills/flowd/" + name, data: []byte(projectFile(f.t, "skills/flowd/"+name))})
	}
	return entries
}

func (f *releaseFixture) release(version string, mutate func([]archiveEntry) []archiveEntry) {
	f.t.Helper()
	entries := f.entries(version)
	if mutate != nil {
		entries = mutate(entries)
	}
	directory := filepath.Join(f.assets, "v"+version)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		f.t.Fatal(err)
	}
	var checksums strings.Builder
	for _, platform := range []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"} {
		var buffer bytes.Buffer
		// Compression ratio is not under test; avoid race-instrumented deflate work.
		gzipWriter, err := gzip.NewWriterLevel(&buffer, gzip.NoCompression)
		if err != nil {
			f.t.Fatal(err)
		}
		writer := tar.NewWriter(gzipWriter)
		for _, entry := range entries {
			kind := entry.kind
			if kind == 0 {
				kind = tar.TypeReg
			}
			size := int64(len(entry.data))
			if kind != tar.TypeReg {
				size = 0
			}
			if err := writer.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o644, Size: size, Typeflag: kind, Linkname: entry.link}); err != nil {
				f.t.Fatal(err)
			}
			if size > 0 {
				if _, err := writer.Write(entry.data); err != nil {
					f.t.Fatal(err)
				}
			}
		}
		if err := writer.Close(); err != nil {
			f.t.Fatal(err)
		}
		if err := gzipWriter.Close(); err != nil {
			f.t.Fatal(err)
		}
		name := "flowd_" + version + "_" + platform + ".tar.gz"
		if err := os.WriteFile(filepath.Join(directory, name), buffer.Bytes(), 0o600); err != nil {
			f.t.Fatal(err)
		}
		digest := sha256.Sum256(buffer.Bytes())
		fmt.Fprintf(&checksums, "%s  %s\n", hex.EncodeToString(digest[:]), name)
	}
	if err := os.WriteFile(filepath.Join(directory, "checksums.txt"), []byte(checksums.String()), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// Source a test-only copy without its final entrypoint; production has no test bypass flag.
func (f *releaseFixture) validate(args ...string) (string, error) {
	f.t.Helper()
	source := projectFile(f.t, "install.sh")
	if !strings.HasSuffix(source, "main \"$@\"\n") {
		f.t.Fatal("unexpected installer entrypoint")
	}
	library := filepath.Join(f.root, "installer-library.sh")
	if err := os.WriteFile(library, []byte(strings.TrimSuffix(source, "main \"$@\"\n")), 0o600); err != nil {
		f.t.Fatal(err)
	}
	command := exec.Command("/bin/sh", append([]string{"-c", `. "$1"; shift; prepare_install "$@"`, "fixture", library}, args...)...)
	command.Env = f.environment
	output, err := command.CombinedOutput()
	return string(output), err
}

func (f *releaseFixture) install(args ...string) (string, error) {
	f.t.Helper()
	command := exec.Command("/bin/sh", append([]string{filepath.Join(projectRoot(f.t), "install.sh")}, args...)...)
	command.Env = f.environment
	command.Dir = f.root
	output, err := command.CombinedOutput()
	return string(output), err
}

func TestReleaseFixture(t *testing.T) {
	f := newReleaseFixture(t)
	output, err := exec.Command(f.binary, "--json", "--version").CombinedOutput()
	if err != nil || string(output) != "{\"version\":\"0.1.0\"}\n" {
		t.Fatalf("fixture version: %v; %s", err, output)
	}
	for _, entry := range f.entries("0.1.0") {
		if len(entry.data) == 0 {
			t.Errorf("fixture member %s is empty", entry.name)
		}
	}
	if _, err := os.Stat(filepath.Join(f.home, "data")); !os.IsNotExist(err) {
		t.Error("fixture discovery touched planner data")
	}
}
