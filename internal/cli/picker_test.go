package cli

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// press stands in for the terminal: it records what the picker offers and
// presses the session's keys in it.
func (s *session) press(m tea.Model) (tea.Model, error) {
	switch p := m.(type) {
	case picker:
		for _, item := range p.items {
			s.offered = append(s.offered, item.Title)
		}
	case carryPicker:
		for _, item := range p.items {
			s.offered = append(s.offered, item.Item.Title)
		}
	case orderPicker:
		for _, item := range p.items {
			s.offered = append(s.offered, item.Title)
		}
	}
	for _, k := range s.keys {
		var msg tea.KeyPressMsg
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "space":
			msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
		case "ctrl+s":
			msg = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		m, _ = m.Update(msg)
	}
	return m, nil
}

func TestThePickerStartsTheChosenItem(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Deep work")
	s.ok("add", "30m", "Inbox")
	s.ok("add", "20m", "Notes")
	s.ok("start", s.id("Deep work"))
	s.keys = []string{"down", "enter"}
	out := s.ok("start")
	if strings.Join(s.offered, ", ") != "Inbox, Notes" {
		t.Fatalf("start should only offer todo items: %v", s.offered)
	}
	if !contains(out, "│ Notes │") || !contains(out, "IN PROGRESS") {
		t.Fatalf("got %q", out)
	}
}

func TestCancellingThePickerChangesNothing(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Deep work")
	s.keys = []string{"esc"}
	if out := s.ok("start"); out != "" {
		t.Fatalf("got %q", out)
	}
	s.keys = nil
	if list := s.ok("list"); !contains(list, "TODO") {
		t.Fatalf("nothing should have started:\n%s", list)
	}
}

// Only a person at a terminal is asked. Scripts, pipes and --json still need
// the ID, so nothing waits for an answer that cannot come.
func TestOnlyAPersonIsAskedToPick(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Deep work")
	s.fails(2, "ERR_USAGE", "start")
	s.keys = []string{"enter"}
	s.fails(2, "ERR_USAGE", "--json", "start")
	if len(s.offered) != 0 {
		t.Fatal("--json must never open the picker")
	}
	_, errOut, code := s.run("pause")
	if code != 1 || errOut != "Error: Nothing on Tue 17 Nov 2026 can be paused.\n       See the day with flowd list.\n" {
		t.Fatalf("exit %d: %q", code, errOut)
	}
}

func TestThePickerMovesAndWraps(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "First")
	s.ok("add", "1h", "Second")
	s.keys = []string{"up", "enter"}
	if out := s.ok("start"); !contains(out, "│ Second │") {
		t.Fatalf("up from the top should wrap to the last item: %q", out)
	}
	// Starting never moves an item, so First is still the first row.
	s.keys = []string{"j", "k", "enter"}
	if out := s.ok("delete", "--yes"); !contains(out, "│ First │") {
		t.Fatalf("j then k should come back to the first row: %q", out)
	}
}

func TestThePickerDrawsTheItemsAndLeavesNothingBehind(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Deep work")
	s.ok("add", "30m", "Inbox")
	var drawn string
	s.keys = []string{}
	s.picker = func(m tea.Model) (tea.Model, error) {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		drawn = ansi.Strip(m.View().Content)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.View().Content != "" {
			t.Error("a closed picker should leave nothing behind")
		}
		return m, nil
	}
	s.ok("start")
	if !contains(drawn, "Start which item?") || !contains(drawn, "> Inbox") || !contains(drawn, "  Deep work") {
		t.Fatalf("got:\n%s", drawn)
	}
}
