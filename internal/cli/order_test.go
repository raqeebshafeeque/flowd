package cli

import (
	"strings"
	"testing"
)

func titlesOf(t *testing.T, s *session) string {
	t.Helper()
	var names []string
	for _, row := range strings.Split(s.ok("list"), "\n") {
		if strings.HasPrefix(row, "│ it_") {
			names = append(names, strings.TrimSpace(strings.Split(row, "│")[2]))
		}
	}
	return strings.Join(names, ", ")
}

func TestOrderTakesTheWholeDay(t *testing.T) {
	s := newSession(t)
	for _, title := range []string{"A", "B", "C"} {
		s.ok("add", "30m", title)
	}
	a, b, c := s.id("A"), s.id("B"), s.id("C")
	if out := s.ok("order", c, a, b); !strings.HasPrefix(out, "Tue 17 Nov 2026\n┌") {
		t.Fatalf("got %q", out)
	}
	if got := titlesOf(t, s); got != "C, A, B" {
		t.Fatalf("got %s", got)
	}
	s.ok("start", a)
	if got := titlesOf(t, s); got != "C, A, B" {
		t.Fatalf("starting moved an item: %s", got)
	}
	s.fails(1, "ERR_STALE", "order", a, b)
	s.fails(2, "ERR_USAGE", "order")
}

// At a terminal the day is arranged with j/k and J/K, and only Ctrl+S saves.
func TestOrderAtATerminal(t *testing.T) {
	s := newSession(t)
	for _, title := range []string{"A", "B", "C"} {
		s.ok("add", "30m", title)
	}
	s.keys = []string{"J", "J", "esc"}
	if out := s.ok("order"); out != "" {
		t.Fatalf("esc should save nothing: %q", out)
	}
	// A moves to the bottom, then the cursor goes back up to B, which is
	// already first, so K does nothing there.
	s.keys = []string{"J", "J", "k", "k", "K", "ctrl+s"}
	s.ok("order")
	s.keys = nil
	if got := titlesOf(t, s); got != "B, C, A" {
		t.Fatalf("got %s", got)
	}
}
