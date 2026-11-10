package distribution

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fakePackage(ctx context.Context, directory, version string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(directory, "dist"), 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "dist", "VERSION"), []byte(version), 0o600)
}

func testPackageStore(t *testing.T) *packageFixtureStore {
	t.Helper()
	store := &packageFixtureStore{}
	t.Cleanup(func() {
		if err := store.close(); err != nil {
			t.Error(err)
		}
	})
	return store
}

func TestPackageCacheConcurrentRequests(t *testing.T) {
	store := testPackageStore(t)
	var builds atomic.Int32
	var signal sync.Once
	started, release := make(chan struct{}), make(chan struct{})
	build := func(ctx context.Context, directory, version string) error {
		builds.Add(1)
		signal.Do(func() { close(started) })
		select {
		case <-release:
			return fakePackage(ctx, directory, version)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	type result struct {
		path string
		err  error
	}
	const consumers = 16
	results := make(chan result, consumers)
	for range consumers {
		go func() {
			path, err := store.load("0.1.0", build)
			results <- result{path, err}
		}()
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("package preparation did not start")
	}
	select {
	case value := <-results:
		close(release)
		t.Fatalf("partial fixture exposed before build completion: %+v", value)
	default:
	}
	close(release)
	var master string
	for range consumers {
		value := <-results
		if value.err != nil {
			t.Fatal(value.err)
		}
		if master == "" {
			master = value.path
		}
		if value.path != master {
			t.Error("same-version consumers received different masters")
		}
		contents, err := os.ReadFile(filepath.Join(value.path, "dist", "VERSION"))
		if err != nil || string(contents) != "0.1.0" {
			t.Errorf("incomplete package: %q; %v", contents, err)
		}
	}
	if got := builds.Load(); got != 1 {
		t.Errorf("same-version build count = %d, want 1", got)
	}
}

func TestPackageCacheVersions(t *testing.T) {
	store := testPackageStore(t)
	counts, masters := map[string]int{}, map[string]string{}
	build := func(ctx context.Context, directory, version string) error {
		counts[version]++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 3*time.Minute {
			return errors.New("package build must have a bounded context")
		}
		return fakePackage(ctx, directory, version)
	}
	for _, version := range []string{"0.1.0", "0.2.0", "0.1.0", "0.2.0"} {
		master, err := store.load(version, build)
		if err != nil {
			t.Fatal(err)
		}
		if previous := masters[version]; previous != "" && previous != master {
			t.Errorf("version %s changed masters", version)
		}
		masters[version] = master
		if info, err := os.Stat(master); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("master is not private: %v; %v", info, err)
		}
		contents, err := os.ReadFile(filepath.Join(master, "dist", "VERSION"))
		if err != nil || string(contents) != version {
			t.Errorf("version %s package contents = %q; %v", version, contents, err)
		}
	}
	if masters["0.1.0"] == masters["0.2.0"] {
		t.Error("different versions share a master")
	}
	for version, count := range counts {
		if count != 1 {
			t.Errorf("version %s build count = %d, want 1", version, count)
		}
	}
}

func TestPackageCacheFailure(t *testing.T) {
	store := testPackageStore(t)
	cause := errors.New("fixture disk failure")
	pathError := &os.PathError{Op: "write", Path: "partial archive", Err: cause}
	builds := 0
	build := func(ctx context.Context, directory, version string) error {
		builds++
		if err := fakePackage(ctx, directory, version); err != nil {
			return err
		}
		return pathError
	}
	for range 3 {
		path, err := store.load("0.1.0", build)
		if path != "" || !errors.Is(err, cause) {
			t.Errorf("partial path %q or lost build failure: %v", path, err)
		}
		var got *os.PathError
		if !errors.As(err, &got) || got != pathError {
			t.Errorf("lost underlying path error: %v", err)
		}
	}
	if builds != 1 {
		t.Errorf("failed build attempts = %d, want 1", builds)
	}
	if _, err := store.load("0.2.0", fakePackage); err != nil {
		t.Fatalf("another version must remain usable: %v", err)
	}
}

func TestPackageCacheOutlivesFirstConsumer(t *testing.T) {
	store := testPackageStore(t)
	var first string
	t.Run("first consumer", func(t *testing.T) {
		var err error
		first, err = store.load("0.1.0", fakePackage)
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("later consumer", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(first, "dist", "VERSION")); err != nil {
			t.Fatalf("fixture disappeared with first consumer: %v", err)
		}
		if _, err := store.load("0.1.0", fakePackage); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPackageCachePrivateCheckouts(t *testing.T) {
	store := testPackageStore(t)
	build := func(ctx context.Context, directory, version string) error {
		if err := fakePackage(ctx, directory, version); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(directory, ".release-please-manifest.json"), []byte("original metadata"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(directory, "flowd"), []byte("executable fixture"), 0o755)
	}
	var first string
	t.Run("mutating consumer", func(t *testing.T) {
		var err error
		first, err = store.checkout("0.1.0", build, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{".release-please-manifest.json", "dist/VERSION"} {
			if err := os.WriteFile(filepath.Join(first, path), []byte("changed"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("later consumer", func(t *testing.T) {
		second, err := store.checkout("0.1.0", build, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if second == first {
			t.Error("consumers share a mutable checkout")
		}
		master, err := store.load("0.1.0", build)
		if err != nil {
			t.Fatal(err)
		}
		for _, root := range []string{master, second} {
			for path, want := range map[string]string{".release-please-manifest.json": "original metadata", "dist/VERSION": "0.1.0"} {
				contents, err := os.ReadFile(filepath.Join(root, path))
				if err != nil || string(contents) != want {
					t.Errorf("%s inherited another consumer's mutation: %q; %v", path, contents, err)
				}
			}
			info, err := os.Stat(filepath.Join(root, "flowd"))
			if err != nil || info.Mode().Perm() != 0o755 {
				t.Errorf("executable mode lost: %v; %v", info, err)
			}
		}
	})
}

func TestPackageToolAvailability(t *testing.T) {
	for _, test := range []struct {
		name string
		ci   string
		skip bool
	}{
		{"optional local tool", "", true},
		{"explicit local environment", "false", true},
		{"required CI tool", "true", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestNoPublishBuild$", "-test.v")
			command.Env = append(os.Environ(), "FLOWD_GORELEASER=", "CI="+test.ci, "PATH="+t.TempDir())
			output, err := command.CombinedOutput()
			if test.skip {
				if err != nil || !strings.Contains(string(output), "--- SKIP") {
					t.Fatalf("optional local tool must skip explicitly: %v; %s", err, output)
				}
			} else if err == nil || strings.Contains(string(output), "--- SKIP") {
				t.Fatalf("required CI tool must fail, not skip: %v; %s", err, output)
			}
		})
	}
}

func TestPackageBuilderDoesNotPublishOrReceiveTokens(t *testing.T) {
	for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN", "GITLAB_TOKEN", "GITEA_TOKEN", "RELEASE_PLEASE_TOKEN"} {
		t.Setenv(name, "fixture-only-token")
	}
	root := t.TempDir()
	tool := filepath.Join(root, "goreleaser")
	script := "#!/bin/sh\nset -eu\n" +
		"[ \"$#\" -eq 3 ] && [ \"$1\" = release ] && [ \"$2\" = --skip=publish ] && [ \"$3\" = --clean ]\n" +
		"[ -z \"${GITHUB_TOKEN:-}${GH_TOKEN:-}${GITLAB_TOKEN:-}${GITEA_TOKEN:-}${RELEASE_PLEASE_TOKEN:-}\" ]\n" +
		"printf checked > fixture-verified\n"
	if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "checkout")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := buildPackageFixture(ctx, projectRoot(t), tool, directory, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	if contents, err := os.ReadFile(filepath.Join(directory, "fixture-verified")); err != nil || string(contents) != "checked" {
		t.Fatalf("controlled no-publish tool did not run: %q; %v", contents, err)
	}
}

func TestPackageCacheProcessHelper(t *testing.T) {
	record := os.Getenv("FLOWD_TEST_PACKAGE_CACHE_RECORD")
	if record == "" {
		t.Skip("suite cleanup subprocess helper")
	}
	master, err := suitePackages.load("0.1.0", fakePackage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, []byte(master), 0o600); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("FLOWD_TEST_PACKAGE_CACHE_FAIL") == "true" {
		t.Error("requested suite failure")
	}
}

func TestPackageCacheSuiteCleanup(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run("failed suite="+strconv.FormatBool(fail), func(t *testing.T) {
			record := filepath.Join(t.TempDir(), "master")
			command := exec.Command(os.Args[0], "-test.run=^TestPackageCacheProcessHelper$")
			command.Env = append(os.Environ(), "FLOWD_TEST_PACKAGE_CACHE_RECORD="+record,
				"FLOWD_TEST_PACKAGE_CACHE_FAIL="+strconv.FormatBool(fail))
			output, err := command.CombinedOutput()
			if (err != nil) != fail {
				t.Fatalf("suite exit status: %v; %s", err, output)
			}
			master, err := os.ReadFile(record)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(string(master)); !os.IsNotExist(err) {
				t.Errorf("suite left its master behind: %v", err)
			}
		})
	}
}
