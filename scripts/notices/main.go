// Command notices prints the upstream licenses for the four release targets.
// Run from the repository root: go run ./scripts/notices > THIRD_PARTY_NOTICES.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type module struct {
	Path, Version, Dir string
	Main               bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	modules := map[string]module{}
	for _, target := range []struct{ os, arch string }{
		{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"},
	} {
		command := exec.Command("go", "list", "-mod=mod", "-deps", "-json", "./cmd/flowd")
		command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target.os, "GOARCH="+target.arch)
		var stderr bytes.Buffer
		command.Stderr = &stderr
		output, err := command.Output()
		if err != nil {
			return fmt.Errorf("list %s/%s modules: %w: %s", target.os, target.arch, err, &stderr)
		}
		decoder := json.NewDecoder(bytes.NewReader(output))
		for {
			var pkg struct{ Module *module }
			if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return err
			}
			if pkg.Module != nil && !pkg.Module.Main {
				modules[pkg.Module.Path] = *pkg.Module
			}
		}
	}
	var result bytes.Buffer
	fmt.Fprintln(&result, "Third-party notices for flowd")
	fmt.Fprintln(&result, "\nThird-party components retain their own licenses, not flowd's MIT license.")
	fmt.Fprintln(&result, "The upstream texts below are preserved verbatim. Upstream notice bundles can")
	fmt.Fprintln(&result, "also describe optional components. Regenerate after changing Go or dependencies:")
	fmt.Fprintln(&result, "\n    go run ./scripts/notices > THIRD_PARTY_NOTICES")
	if err := appendLicenses(&result, "Go standard library and runtime", runtime.GOROOT()); err != nil {
		return err
	}
	paths := make([]string, 0, len(modules))
	for path := range modules {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		m := modules[path]
		if err := appendLicenses(&result, m.Path+" "+m.Version, m.Dir); err != nil {
			return err
		}
	}
	_, err := os.Stdout.Write(result.Bytes())
	return err
}

func appendLicenses(result *bytes.Buffer, label, directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read %s license directory: %w", label, err)
	}
	found := false
	for _, entry := range entries {
		name := strings.ToUpper(entry.Name())
		if entry.IsDir() || !(strings.HasPrefix(name, "LICENSE") || strings.HasPrefix(name, "COPYING") || strings.HasPrefix(name, "NOTICE") || name == "PATENTS") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return fmt.Errorf("read %s/%s: %w", label, entry.Name(), err)
		}
		if len(bytes.TrimSpace(content)) == 0 {
			return fmt.Errorf("empty license %s/%s", label, entry.Name())
		}
		fmt.Fprintf(result, "\n======================================================================\n%s — %s\n======================================================================\n\n", label, entry.Name())
		result.Write(content)
		if content[len(content)-1] != '\n' {
			result.WriteByte('\n')
		}
		found = true
	}
	if !found {
		return fmt.Errorf("no upstream license found for %s", label)
	}
	return nil
}
