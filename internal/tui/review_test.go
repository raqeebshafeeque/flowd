package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

func TestTheReviewMatchesTheDay(t *testing.T) {
	w := newWorkspace(t)
	a := w.add("Study", "1h")
	w.add("Code review", "30m")
	w.act(planner.Start, a)
	w.now = w.now.Add(40 * time.Minute)
	w.act(planner.Finish, a)
	w.refresh()
	w.m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	w.press(key("r"))
	if !w.m.review {
		t.Fatal("r should open the review")
	}
	text := plain(w)
	for _, want := range []string{"Summary", "Total duration   1h30m", "Recorded work    40m", "Progress", "1 / 2 finished", "Status", "TODO (1)", "• Code review", "DONE (1)", "• Study", "INCOMPLETE (0)", "back [esc]"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	w.press(key("esc"))
	if w.m.review {
		t.Fatal("esc should close the review")
	}
}

func TestTheReviewScrollsWithinItsLength(t *testing.T) {
	w := newWorkspace(t)
	for i := 0; i < 20; i++ {
		w.add("Item", "10m")
	}
	w.refresh()
	w.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	w.press(key("r"))
	w.press(key("k"))
	if w.m.reviewOffset != 0 {
		t.Fatal("scrolling above the top must do nothing")
	}
	for i := 0; i < 100; i++ {
		w.press(key("j"))
	}
	w.m.View()
	text := plain(w)
	if !strings.Contains(text, "INCOMPLETE (0)") || strings.Contains(text, "Summary") {
		t.Fatalf("scrolling down should reach the end and stop there:\n%s", text)
	}
	for i, row := range screen(w) {
		if ansi.StringWidth(row) != 80 {
			t.Fatalf("row %d is %d wide", i, ansi.StringWidth(row))
		}
	}
}

func TestTheReviewChangesNothing(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	w.refresh()
	w.press(key("r"))
	for _, k := range []tea.KeyPressMsg{space(), key("f"), key("s"), key("n"), key("e")} {
		if cmd := w.press(k); cmd != nil {
			t.Fatalf("%s did something inside the review", k.String())
		}
	}
	if w.m.form != nil || w.status(id) != "todo" {
		t.Fatal("the review changed something")
	}
	if !quits(w.press(key("q"))) {
		t.Fatal("q should still quit")
	}
}

func TestAnEmptyDayReviewsAsZero(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("r"))
	text := plain(w)
	if !strings.Contains(text, "Total duration   0m") || !strings.Contains(text, "0 / 0 finished") {
		t.Fatalf("got:\n%s", text)
	}
}

func TestSectionsAreSeparatedByTwoEmptyRows(t *testing.T) {
	w := newWorkspace(t)
	w.add("Study", "1h")
	w.refresh()
	for _, page := range []string{"r", "?"} {
		w.m.review, w.m.help = false, false
		w.press(key(page))
		rows := w.m.reviewRows(78)
		if page == "?" {
			rows = w.m.helpRows(78)
		}
		var headings []int
		for i := range rows {
			if i+1 < len(rows) && strings.Contains(ansi.Strip(rows[i+1]), "────") {
				headings = append(headings, i)
			}
		}
		if len(headings) < 3 || headings[0] != 1 || strings.TrimSpace(rows[0]) != "" {
			t.Fatalf("%s: the first section should follow one empty row: %v", page, headings)
		}
		for _, h := range headings[1:] {
			if strings.TrimSpace(rows[h-1]) != "" || strings.TrimSpace(rows[h-2]) != "" || strings.TrimSpace(ansi.Strip(rows[h-3])) == "" {
				t.Fatalf("%s: section at row %d does not have exactly two empty rows above it", page, h)
			}
		}
	}
}
