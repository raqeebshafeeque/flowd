package cli

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/planner"
)

func TestParse(t *testing.T) {
	cases := []struct {
		args    []string
		command string
		check   func(options) bool
	}{
		{nil, "list", func(o options) bool { return o.date == "" }},
		{[]string{"--db", "x.sqlite", "--json", "review"}, "review", func(o options) bool { return o.db == "x.sqlite" && o.json }},
		{[]string{"add", "1h", "Learn", "Go"}, "add", func(o options) bool { return len(o.args) == 3 }},
		{[]string{"edit", "--title", "New", "2"}, "edit", func(o options) bool { return *o.title == "New" && o.duration == nil && o.args[0] == "2" }},
		{[]string{"doctor", "--repair"}, "doctor", func(o options) bool { return o.repair }},
	}
	for _, c := range cases {
		o, err := parse(c.args)
		if err != nil || o.command != c.command || !c.check(o) {
			t.Errorf("%v: got %+v, %v", c.args, o, err)
		}
	}
}

func TestParseRefusesBadCommandLines(t *testing.T) {
	bad := [][]string{
		{"dance"},
		{"add", "1h"},
		{"start", "1", "2"},
		{"completion"},
		{"list", "extra"},
		{"edit", "1"},
		{"--date", "2026-02-30"},
		{"2026-11-18"},
		{"skip", "it_1"},
		{"events", "it_1"},
		{"--timezone", "UTC"},
		{"now", "--max", "9"},
		{"--nope"},
		{"start", "--nope", "1"},
	}
	for _, args := range bad {
		if _, err := parse(args); !errors.Is(err, planner.ErrInput) {
			t.Errorf("%v was accepted: %v", args, err)
		}
	}
}

func TestHelpMentionsEveryCommand(t *testing.T) {
	for _, name := range commandNames() {
		if !contains(help, name) {
			t.Errorf("help does not mention %s", name)
		}
	}
}

// The Agent Skill's command reference must document exactly the commands
// flowd has, so agents never guess.
func TestSkillReferenceMatchesTheCommands(t *testing.T) {
	raw, err := os.ReadFile("../../skills/flowd/references/commands.md")
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if name, ok := strings.CutPrefix(line, "### "); ok {
			documented[name] = true
		}
	}
	for _, name := range commandNames() {
		if !documented[name] {
			t.Errorf("the skill does not document %s", name)
		}
		delete(documented, name)
	}
	for name := range documented {
		t.Errorf("the skill documents %s, which flowd does not have", name)
	}
	skill, err := os.ReadFile("../../skills/flowd/SKILL.md")
	if err != nil || !strings.HasPrefix(string(skill), "---\nname: flowd\ndescription: ") || !strings.Contains(string(skill), "(references/commands.md)") {
		t.Fatal("the skill needs its front matter and a link to the reference", err)
	}
}
