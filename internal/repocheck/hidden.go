package repocheck

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// checkedExtensions are the text files a person reads or a tool runs.
var checkedExtensions = map[string]bool{".go": true, ".md": true, ".sh": true, ".yml": true, ".yaml": true, ".json": true, ".sql": true}

// HiddenCharacters reports every invisible, direction-changing, private-use
// or control character in the repository's source and documentation, which
// could make code read differently from how it runs. Tests that need such
// characters write them as escapes like \u202e instead.
func HiddenCharacters(root string) ([]string, error) {
	var problems []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (skipped[entry.Name()] || strings.HasPrefix(entry.Name(), ".")) && entry.Name() != ".github" {
				return filepath.SkipDir
			}
			return nil
		}
		if !checkedExtensions[filepath.Ext(path)] {
			return nil
		}
		found, err := hiddenIn(path)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		for _, f := range found {
			problems = append(problems, relative+":"+f)
		}
		return nil
	})
	sort.Strings(problems)
	return problems, err
}

func hiddenIn(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var found []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for line := 1; scanner.Scan(); line++ {
		for _, r := range scanner.Text() {
			if r != '\t' && (unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co) || r == '\u2028' || r == '\u2029') {
				found = append(found, fmt.Sprintf("%d: U+%04X", line, r))
			}
		}
	}
	return found, scanner.Err()
}
