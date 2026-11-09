// Package releasecheck validates downloaded distribution assets before publication.
package releasecheck

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var normalVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var members = []string{"flowd", "VERSION", "LICENSE", "skills/flowd/SKILL.md", "skills/flowd/VERSION", "skills/flowd/references/commands.md", "skills/flowd/references/safety.md"}

// Verify checks the complete release and smokes only the native executable.
// Checksums provide same-channel integrity, not independent publisher authentication.
func Verify(directory, version string) error {
	if !normalVersion.MatchString(version) {
		return fmt.Errorf("invalid normal release version %q", version)
	}
	expected := map[string]bool{"install.sh": true}
	for _, target := range []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"} {
		expected["flowd_"+version+"_"+target+".tar.gz"] = true
	}
	checksums, err := os.ReadFile(filepath.Join(directory, "checksums.txt"))
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(checksums)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("malformed checksum entry")
		}
		name := fields[1]
		if !expected[name] || seen[name] {
			return fmt.Errorf("unexpected or duplicate checksum %q", name)
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size {
			return fmt.Errorf("invalid checksum for %s", name)
		}
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular asset %s", name)
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), fields[0]) {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
		seen[name] = true
	}
	for name := range expected {
		if !seen[name] {
			return fmt.Errorf("missing checksum for %s", name)
		}
	}
	var native []byte
	nativeName := "flowd_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	for name := range expected {
		if !strings.HasSuffix(name, ".tar.gz") {
			continue
		}
		files, err := readArchive(filepath.Join(directory, name))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, record := range []string{"VERSION", "skills/flowd/VERSION"} {
			if string(files[record]) != version+"\n" {
				return fmt.Errorf("%s: wrong %s", name, record)
			}
		}
		skill := string(files["skills/flowd/SKILL.md"])
		if !strings.Contains(skill, "\nname: flowd\n") || !strings.Contains(skill, "\ndescription: ") {
			return fmt.Errorf("%s: invalid skill metadata", name)
		}
		for _, reference := range []string{"references/commands.md", "references/safety.md"} {
			if !strings.Contains(skill, "("+reference+")") {
				return fmt.Errorf("%s: missing skill reference %s", name, reference)
			}
		}
		if name == nativeName {
			native = files["flowd"]
		}
	}
	if native == nil {
		return fmt.Errorf("cannot smoke unsupported native platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return smoke(native, version)
}

func readArchive(path string) (map[string][]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	files := map[string][]byte{}
	allowed := map[string]bool{}
	for _, name := range members {
		allowed[name] = true
	}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if !allowed[header.Name] || files[header.Name] != nil || header.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("unsupported, duplicate or non-regular member %q", header.Name)
		}
		if header.Size <= 0 || header.Size > 64<<20 {
			return nil, fmt.Errorf("invalid member size for %s", header.Name)
		}
		contents, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		files[header.Name] = contents
	}
	for _, name := range members {
		if files[name] == nil {
			return nil, fmt.Errorf("missing member %s", name)
		}
	}
	return files, nil
}

func smoke(binary []byte, version string) error {
	file, err := os.CreateTemp("", "flowd-release-check-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(binary); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(0o700); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	sandbox, err := os.MkdirTemp("", "flowd-release-home-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(sandbox)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, mode := range []string{"text", "json", "help"} {
		args := []string{"--version"}
		if mode == "json" {
			args = []string{"--json", "--version"}
		}
		if mode == "help" {
			args = []string{"--help"}
		}
		command := exec.CommandContext(ctx, file.Name(), args...)
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + sandbox, "XDG_DATA_HOME=" + filepath.Join(sandbox, "data"), "TMPDIR=" + sandbox, "TZ=UTC"}
		output, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("native %s smoke: %w", mode, err)
		}
		switch mode {
		case "text":
			if string(output) != "flowd "+version+"\n" {
				return fmt.Errorf("native executable version mismatch")
			}
		case "json":
			var result struct {
				Version string `json:"version"`
			}
			if err := json.Unmarshal(output, &result); err != nil {
				return err
			}
			if result.Version != version {
				return fmt.Errorf("native JSON version mismatch")
			}
		case "help":
			if !strings.Contains(string(output), "Usage: flowd") {
				return fmt.Errorf("native help missing")
			}
		}
	}
	return nil
}
