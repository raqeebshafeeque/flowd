package repocheck

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRepositoryHasNoHiddenCharacters(t *testing.T) {
	problems, err := HiddenCharacters(repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

func TestHiddenCharactersAreFound(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"code.go":       "package x\n// fine\nvar s = \"evil\u202etxt\"\n",
		"notes.md":      "zero\u200bwidth\n",
		"clean.md":      "Plain text, emoji 👍 and é are fine\n",
		"image.png":     "\u202e is not checked in binary files",
		"testdata/x.go": "\u202e is test data\n",
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	problems, err := HiddenCharacters(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"code.go:3: U+202E", "notes.md:1: U+200B"}
	if !reflect.DeepEqual(problems, want) {
		t.Fatalf("got %q, want %q", problems, want)
	}
}
