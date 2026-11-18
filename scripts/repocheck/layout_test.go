package repocheck

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRepositoryLayout(t *testing.T) {
	problems, err := Layout(repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

func TestLayoutReportsBrokenRules(t *testing.T) {
	root := t.TempDir()
	write := func(path string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("good/doc.go")
	write("good/calendar.go")
	write("good/calendar_test.go")
	write("nodoc/item.go")
	write("stray/doc.go")
	write("stray/helpers_test.go")
	write(".hidden/ignored.go")
	write("testdata/ignored.go")

	problems, err := Layout(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"nodoc: missing doc.go",
		"stray: helpers_test.go has no matching helpers.go",
	}
	if !reflect.DeepEqual(problems, want) {
		t.Fatalf("got %q, want %q", problems, want)
	}
}

// repositoryRoot finds the directory holding go.mod above this package.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
