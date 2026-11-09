package distribution

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func projectFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(projectRoot(t), path))
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func TestSkillPreflight(t *testing.T) {
	text := projectFile(t, "skills/flowd/SKILL.md")
	for _, rule := range []string{"command -v flowd", "flowd --json --version", "VERSION", "mismatch", "no writes", "developer", "one-shot", "cancellation"} {
		if !strings.Contains(text, rule) {
			t.Errorf("missing preflight/lifetime rule %q", rule)
		}
	}
}

func TestSkillSafetyCases(t *testing.T) {
	text := projectFile(t, "skills/flowd/references/safety.md")
	for _, name := range []string{"Duplicate titles", "Missing duration", "Stored instructions", "Automatic pause", "Overlap refusal", "Partial completion", "Uncertain copy", "Repair consent"} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(text, "### "+name) {
				t.Error("missing evaluation case")
			}
		})
	}
	if !strings.Contains(projectFile(t, "skills/flowd/SKILL.md"), "(references/safety.md)") {
		t.Error("skill must link safety guidance")
	}
}

func TestSkillCommandCoverage(t *testing.T) {
	reference := projectFile(t, "skills/flowd/references/commands.md")
	source := projectFile(t, "internal/cli/args.go")
	expected := map[string]bool{}
	for _, line := range strings.Split(source, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "case ") {
			continue
		}
		for _, match := range regexp.MustCompile(`"([a-z]+)"`).FindAllStringSubmatch(line, -1) {
			expected[match[1]] = true
		}
	}
	actual := map[string]bool{}
	for _, match := range regexp.MustCompile(`(?m)^### ([a-z]+)$`).FindAllStringSubmatch(reference, -1) {
		actual[match[1]] = true
	}
	for name := range expected {
		if !actual[name] {
			t.Errorf("missing documented command %s", name)
		}
	}
	for name := range actual {
		if !expected[name] {
			t.Errorf("invented command %s", name)
		}
	}
	if !strings.Contains(projectFile(t, "skills/flowd/SKILL.md"), "(references/commands.md)") {
		t.Error("skill must link its command reference")
	}
}

func TestSkillStructure(t *testing.T) {
	text := projectFile(t, "skills/flowd/SKILL.md")
	if !strings.HasPrefix(text, "---\n") {
		t.Fatal("skill needs frontmatter")
	}
	pieces := strings.SplitN(text, "\n---\n", 2)
	if len(pieces) != 2 {
		t.Fatal("skill needs closed frontmatter")
	}
	if !strings.Contains(pieces[0], "name: flowd\n") {
		t.Error("skill name must match its directory")
	}
	description := regexp.MustCompile(`(?m)^description: (.+)$`).FindStringSubmatch(pieces[0])
	if len(description) != 2 || len(description[1]) < 20 || len(description[1]) > 1024 {
		t.Error("description must explain routing")
	}
	for _, step := range []string{"Read", "Resolve", "Act", "Verify"} {
		if !strings.Contains(pieces[1], step) {
			t.Errorf("missing workflow step %s", step)
		}
	}
	for _, match := range regexp.MustCompile(`\]\(([^)]+)\)`).FindAllStringSubmatch(pieces[1], -1) {
		target := match[1]
		if filepath.IsAbs(target) || strings.Contains(target, "..") || strings.Contains(target, "://") {
			t.Errorf("skill reference must be bundled and relative: %s", target)
		}
		if _, err := os.Stat(filepath.Join(projectRoot(t), "skills/flowd", target)); err != nil {
			t.Error(err)
		}
	}
	if strings.Contains(text, "/Users/") || strings.Contains(text, "go run ") {
		t.Error("normal skill use must not depend on the checkout")
	}
}
