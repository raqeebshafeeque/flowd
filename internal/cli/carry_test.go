package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// leftovers plans two earlier days and comes back to the 17th: two items left
// incomplete on the 15th and one on the 16th.
func leftovers(t *testing.T) *session {
	s := newSession(t)
	s.now = time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	s.ok("add", "1h", "Write release notes")
	s.ok("add", "30m", "Reply to the review")
	s.now = time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	s.ok("add", "2h", "Refactor the scheduler")
	s.now = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	return s
}

func TestThePickerCarriesWhatIsTicked(t *testing.T) {
	s := leftovers(t)
	s.keys = []string{"space", "down", "down", "space", "ctrl+s"}
	out := s.ok("carry")
	if strings.Join(s.offered, ", ") != "Refactor the scheduler, Write release notes, Reply to the review" {
		t.Fatalf("offered %v", s.offered)
	}
	if !strings.HasPrefix(out, "Tue 17 Nov 2026\n┌") || !contains(out, "Refactor the scheduler") || !contains(out, "Reply to the review") || contains(out, "Write release notes") {
		t.Fatalf("got:\n%s", out)
	}
	s.keys = nil
	if list := s.ok("list"); !contains(list, "Refactor the scheduler") || !contains(list, "Reply to the review") {
		t.Fatalf("today:\n%s", list)
	}
}

// Ctrl+S with nothing ticked saves nothing, and Esc carries nothing.
func TestNothingTickedCarriesNothing(t *testing.T) {
	s := leftovers(t)
	s.keys = []string{"esc"}
	if out := s.ok("carry"); out != "" {
		t.Fatalf("got %q", out)
	}
	s.keys = []string{"down", "enter", "ctrl+s", "esc"}
	if out := s.ok("carry"); out != "" {
		t.Fatalf("enter and an empty ctrl+s should carry nothing: %q", out)
	}
	s.keys = []string{"down", "space", "ctrl+s"}
	if out := s.ok("carry"); !contains(out, "Write release notes") || contains(out, "Refactor") {
		t.Fatalf("got %q", out)
	}
}

func TestCarryAllAndByID(t *testing.T) {
	s := leftovers(t)
	id := s.id("Refactor the scheduler", "--date", "2026-11-16")
	if out := s.ok("carry", id); !contains(out, "Refactor the scheduler") {
		t.Fatalf("got %q", out)
	}
	var carried []map[string]any
	if err := json.Unmarshal([]byte(s.ok("--json", "carry", "--all")), &carried); err != nil || len(carried) != 2 {
		t.Fatalf("got %v, %v", carried, err)
	}
	if carried[0]["carried_from"] == nil || carried[0]["status"] != "todo" {
		t.Fatalf("got %v", carried[0])
	}
	if out := s.ok("carry", "--all"); out != "Nothing to carry from the last 7 days.\n" {
		t.Fatalf("got %q", out)
	}
	s.fails(1, "ERR_NOT_CARRYABLE", "carry", id)
}

// Without a terminal, or with --json, the items have to be named.
func TestScriptsNameWhatToCarry(t *testing.T) {
	s := leftovers(t)
	s.fails(2, "ERR_USAGE", "carry")
	s.keys = []string{"enter"}
	s.fails(2, "ERR_USAGE", "--json", "carry")
	if len(s.offered) != 0 {
		t.Fatal("--json must never open the picker")
	}
	s.fails(2, "ERR_USAGE", "carry", "--all", "it_x")
}
