package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// leftovers plans two earlier days and comes back to the 17th: on the 15th
// an item left incomplete and one skipped, on the 16th one left incomplete.
func leftovers(t *testing.T) *session {
	s := newSession(t)
	s.now = time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	s.ok("add", "1h", "Write release notes")
	s.ok("add", "30m", "Reply to the review")
	s.ok("skip", s.id("Reply to the review", "--date", "2026-11-15"), "--date", "2026-11-15")
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

// Every row puts its date, duration and status in the same columns, and the
// cursor and ticks show.
func TestLeftoverRowsLineUp(t *testing.T) {
	s := leftovers(t)
	var drawn []string
	s.keys = []string{}
	s.picker = func(m tea.Model) (tea.Model, error) {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
		drawn = strings.Split(ansi.Strip(m.View().Content), "\n")
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		return m, nil
	}
	s.ok("carry")
	if len(drawn) != 4 || !strings.HasPrefix(drawn[0], "Carry which items to Tue 17 Nov 2026?") {
		t.Fatalf("got:\n%s", strings.Join(drawn, "\n"))
	}
	if !strings.HasPrefix(drawn[1], "> [x] Refactor the scheduler") || !strings.HasPrefix(drawn[2], "  [ ] Write release notes") {
		t.Fatalf("got:\n%s", strings.Join(drawn, "\n"))
	}
	date := strings.Index(drawn[1], "Mon 16 Nov")
	for _, row := range drawn[2:] {
		if strings.Index(row, "Sun 15 Nov") != date {
			t.Fatalf("dates do not line up:\n%s", strings.Join(drawn, "\n"))
		}
		if ansi.StringWidth(row) != ansi.StringWidth(drawn[1]) {
			t.Fatalf("statuses do not end in one column:\n%s", strings.Join(drawn, "\n"))
		}
	}
}
