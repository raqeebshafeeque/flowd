package cli

import (
	"errors"
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
		{[]string{"2026-11-20"}, "list", func(o options) bool { return o.date == "2026-11-20" }},
		{[]string{"--db", "x.sqlite", "--json", "review"}, "review", func(o options) bool { return o.db == "x.sqlite" && o.json }},
		{[]string{"add", "1h", "Learn", "Go"}, "add", func(o options) bool { return len(o.args) == 3 }},
		{[]string{"edit", "--title", "New", "2"}, "edit", func(o options) bool { return *o.title == "New" && o.duration == nil && o.args[0] == "2" }},
		{[]string{"doctor", "--repair"}, "doctor", func(o options) bool { return o.repair }},
		{[]string{"--timezone", "Asia/Kolkata", "start", "it_1"}, "start", func(o options) bool { return o.zone == "Asia/Kolkata" }},
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
		{"start"},
		{"start", "1", "2"},
		{"list", "extra"},
		{"edit", "1"},
		{"--date", "2026-02-30"},
		{"--date", "2026-11-17", "2026-11-18"},
		{"--nope"},
		{"start", "--nope", "1"},
	}
	for _, args := range bad {
		if _, err := parse(args); !errors.Is(err, planner.ErrInput) {
			t.Errorf("%v was accepted: %v", args, err)
		}
	}
}

func TestHelpAndVersionNeedNothingElse(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}, {"add", "--help"}} {
		o, err := parse(args)
		if err != nil || !(o.help || o.version) {
			t.Errorf("%v: %+v, %v", args, o, err)
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
