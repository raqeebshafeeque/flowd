package distribution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type packageBuilder func(context.Context, string, string) error

type packageFixture struct {
	once sync.Once
	path string
	err  error
}

// The suite owns immutable masters; callers later copy them into test-owned workspaces.
type packageFixtureStore struct {
	mu      sync.Mutex
	entries map[string]*packageFixture
	roots   []string
}

var suitePackages packageFixtureStore

func TestMain(m *testing.M) {
	code := m.Run()
	if err := suitePackages.close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func (s *packageFixtureStore) load(version string, build packageBuilder) (string, error) {
	s.mu.Lock()
	if s.entries == nil {
		s.entries = make(map[string]*packageFixture)
	}
	entry := s.entries[version]
	if entry == nil {
		entry = &packageFixture{}
		s.entries[version] = entry
	}
	s.mu.Unlock()
	entry.once.Do(func() {
		directory, err := os.MkdirTemp("", "flowd-package-fixtures-*")
		if err != nil {
			entry.err = fmt.Errorf("create package workspace: %w", err)
			return
		}
		s.mu.Lock()
		s.roots = append(s.roots, directory)
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if err := build(ctx, directory, version); err != nil {
			entry.err = fmt.Errorf("prepare package %s: %w", version, err)
			return
		}
		entry.path = directory
	})
	return entry.path, entry.err
}

func (s *packageFixtureStore) checkout(version string, build packageBuilder, destination string) (string, error) {
	master, err := s.load(version, build)
	if err != nil {
		return "", err
	}
	if err := copyPackageFixture(master, destination); err != nil {
		return "", fmt.Errorf("copy package %s: %w", version, err)
	}
	return destination, nil
}

var errOptionalPackageTool = errors.New("set FLOWD_GORELEASER for actual packaging checks")

func packageTool() (string, error) {
	tool := os.Getenv("FLOWD_GORELEASER")
	if tool != "" {
		path, err := exec.LookPath(tool)
		if err != nil {
			return "", fmt.Errorf("configured packaging tool: %w", err)
		}
		return path, nil
	}
	path, err := exec.LookPath("goreleaser")
	if err == nil {
		return path, nil
	}
	if ci := os.Getenv("CI"); ci != "" && ci != "false" {
		return "", fmt.Errorf("required CI packaging tool: %w", err)
	}
	return "", errOptionalPackageTool
}

func packageBuildEnvironment() []string {
	return append(os.Environ(), "GITHUB_TOKEN=", "GH_TOKEN=", "GITLAB_TOKEN=", "GITEA_TOKEN=", "RELEASE_PLEASE_TOKEN=")
}

func buildPackageFixture(ctx context.Context, source, tool, directory, version string) error {
	list := exec.CommandContext(ctx, "git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	list.Dir = source
	list.Env = packageBuildEnvironment()
	output, err := list.Output()
	if err != nil {
		return fmt.Errorf("list package sources: %w", err)
	}
	for _, name := range bytes.Split(output, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		path := string(name)
		from := filepath.Join(source, path)
		info, err := os.Lstat(from)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported source fixture %s", path)
		}
		to := filepath.Join(directory, path)
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			return err
		}
		if err := copyPackageFile(from, to, info.Mode().Perm()); err != nil {
			return err
		}
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "Fixture"},
		{"config", "user.email", "fixture@example.invalid"},
		{"add", "."},
		{"commit", "-qm", "fixture"},
		{"tag", "v" + version},
		{"remote", "add", "origin", "https://github.com/raqeebshafeeque/flowd.git"},
	} {
		command := exec.CommandContext(ctx, "git", args...)
		command.Dir = directory
		command.Env = packageBuildEnvironment()
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("git package fixture: %w; %s", err, output)
		}
	}
	command := exec.CommandContext(ctx, tool, "release", "--skip=publish", "--clean")
	command.Dir = directory
	command.Env = packageBuildEnvironment()
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("no-publish build: %w; %s", err, output)
	}
	return nil
}

func copyPackageFixture(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported package fixture file %s", path)
		}
		return copyPackageFile(path, target, info.Mode().Perm())
	})
}

func copyPackageFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return errors.Join(err, input.Close())
	}
	_, err = io.Copy(output, input)
	return errors.Join(err, output.Chmod(mode), output.Close(), input.Close())
}

// close runs after all suite consumers have finished, including when tests fail.
func (s *packageFixtureStore) close() error {
	var result error
	for _, root := range s.roots {
		if err := os.RemoveAll(root); err != nil {
			result = errors.Join(result, fmt.Errorf("remove package workspace %s: %w", root, err))
		}
	}
	return result
}
