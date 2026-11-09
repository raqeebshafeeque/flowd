package cli

import (
	"errors"
	"testing"
)

func TestParseArgs(t *testing.T) {
	opts, err := parseArgs([]string{"--db", "notes.sqlite", "--date=2026-10-14", "--timezone", "UTC", "--json", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.command != "list" || opts.path != "notes.sqlite" || opts.date != "2026-10-14" || opts.zone != "UTC" || !opts.json {
		t.Error("global options should retain their values")
	}
	opts, err = parseArgs([]string{"2026-10-14"})
	if err != nil || opts.command != "list" || opts.date != "2026-10-14" {
		t.Error("positional date should select its list")
	}
	opts, err = parseArgs([]string{"list", "--help"})
	if err != nil || !opts.help {
		t.Error("command help should be recognized")
	}
	for _, test := range []struct {
		name string
		args []string
	}{
		{"unknown flag", []string{"--unknown"}},
		{"missing value", []string{"--date"}},
		{"empty path", []string{"--db="}},
		{"bad date", []string{"--date", "2026-02-30"}},
		{"repeated option", []string{"--date", "2026-10-14", "--date", "2026-10-15"}},
		{"bad boolean", []string{"--json=maybe"}},
		{"two dates", []string{"--date", "2026-10-14", "2026-10-15"}},
		{"wrong flag position", []string{"list", "--db", "notes.sqlite"}},
		{"extra positional", []string{"now", "extra"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseArgs(test.args); !errors.Is(err, errUsage) {
				t.Errorf("want usage error, got %v", err)
			}
		})
	}
}
