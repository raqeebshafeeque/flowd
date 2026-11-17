package repocheck

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// skipped holds directories that never contain project Go code.
var skipped = map[string]bool{".git": true, "dist": true, "testdata": true}

// Layout walks the repository at root and returns one problem per broken
// rule, sorted so the output is stable. An empty result means the layout is
// fine.
func Layout(root string) ([]string, error) {
	packages := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (skipped[entry.Name()] || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".go" {
			dir, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			packages[dir] = append(packages[dir], entry.Name())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var problems []string
	for dir, files := range packages {
		problems = append(problems, checkPackage(dir, files)...)
	}
	sort.Strings(problems)
	return problems, nil
}

// checkPackage applies the rules to the Go files of one directory.
func checkPackage(dir string, files []string) []string {
	present := map[string]bool{}
	for _, name := range files {
		present[name] = true
	}
	var problems []string
	if !present["doc.go"] {
		problems = append(problems, fmt.Sprintf("%s: missing doc.go", dir))
	}
	for _, name := range files {
		source, isTest := strings.CutSuffix(name, "_test.go")
		if !isTest {
			continue
		}
		if !present[source+".go"] {
			problems = append(problems, fmt.Sprintf("%s: %s has no matching %s.go", dir, name, source))
		}
	}
	return problems
}
