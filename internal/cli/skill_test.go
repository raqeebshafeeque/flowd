package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheSkillGoesWhereItIsAskedFor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := newSession(t)
	out := s.ok("skill", "--codex")
	if !contains(out, "Installed the flowd skill for Codex in ~/.agents/skills/flowd") || contains(out, "Claude") {
		t.Fatalf("got %q", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); err == nil {
		t.Fatal("only --codex was asked for")
	}
	s.ok("skill", "--claude", "--codex")
	for _, dir := range []string{".claude/skills/flowd", ".agents/skills/flowd"} {
		skill, err := os.ReadFile(filepath.Join(home, dir, "SKILL.md"))
		if err != nil || !strings.HasPrefix(string(skill), skillMarker) {
			t.Fatalf("%s: %v", dir, err)
		}
		if version, _ := os.ReadFile(filepath.Join(home, dir, "VERSION")); string(version) != "1.2.3\n" {
			t.Fatalf("%s: version %q", dir, version)
		}
		if _, err := os.Stat(filepath.Join(home, dir, "references", "commands.md")); err != nil {
			t.Fatal(err)
		}
	}
	s.fails(2, "ERR_USAGE", "skill")
}

// An older flowd skill is replaced whole, including files the old installer
// added, and nothing is left beside it.
func TestTheSkillReplacesAnOlderCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	old := filepath.Join(home, ".agents", "skills", "flowd")
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, "SKILL.md"), []byte(skillMarker+"description: old\n---\n"), 0o644)
	os.WriteFile(filepath.Join(old, ".flowd-install"), []byte("flowd-install-v1\n"), 0o600)
	s := newSession(t)
	s.ok("skill", "--codex")
	if _, err := os.Stat(filepath.Join(old, ".flowd-install")); err == nil {
		t.Fatal("the old copy should be replaced whole")
	}
	if skill, _ := os.ReadFile(filepath.Join(old, "SKILL.md")); contains(string(skill), "description: old") {
		t.Fatal("the skill was not updated")
	}
	entries, _ := os.ReadDir(filepath.Dir(old))
	if len(entries) != 1 {
		t.Fatalf("something was left beside the skill: %v", entries)
	}
}

// A folder called flowd that holds anything else is never touched.
func TestTheSkillNeverReplacesSomethingElse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mine := filepath.Join(home, ".claude", "skills", "flowd")
	os.MkdirAll(mine, 0o755)
	os.WriteFile(filepath.Join(mine, "SKILL.md"), []byte("---\nname: something-else\n---\n"), 0o644)
	s := newSession(t)
	_, errOut, code := s.run("skill", "--claude")
	if code != 2 || !contains(errOut, "holds something that is not the flowd skill") {
		t.Fatalf("exit %d: %q", code, errOut)
	}
	if skill, _ := os.ReadFile(filepath.Join(mine, "SKILL.md")); !contains(string(skill), "something-else") {
		t.Fatal("the folder was changed")
	}
	link := filepath.Join(home, ".agents", "skills", "flowd")
	os.MkdirAll(filepath.Dir(link), 0o755)
	os.Symlink(mine, link)
	if _, _, code := s.run("skill", "--codex"); code != 2 {
		t.Fatalf("a symbolic link should be refused, exit %d", code)
	}
}
