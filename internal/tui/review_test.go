package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// openedReview is a workspace with a worked day and the review open on it.
func openedReview(t *testing.T) *workspace {
	w := newWorkspace(t)
	a := w.add("Study", "1h")
	w.add("Code review", "30m")
	w.act(planner.Start, a)
	w.now = w.now.Add(40 * time.Minute)
	w.act(planner.Finish, a)
	w.refresh()
	w.m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	w.run(w.press(key("r")))
	return w
}

func TestTheReviewShowsHowTheDayWent(t *testing.T) {
	w := openedReview(t)
	if !w.m.review {
		t.Fatal("r should open the review")
	}
	text := plain(w)
	for _, want := range []string{"[Day]   Week    Month", "Tuesday 17 November 2026", "Focused      40m", "Sessions     1",
		"Completed", "50%   1 of 2 items", "Worked", "40m of 1h30m planned", "Timeline", "Study", "Code review", "█ working", "back [esc]"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	w.press(key("esc"))
	if w.m.review {
		t.Fatal("esc should close the review")
	}
}

func TestTheReviewSwitchesBetweenDayWeekAndMonth(t *testing.T) {
	w := openedReview(t)
	w.run(w.press(key("tab")))
	if text := plain(w); !strings.Contains(text, "[Week]") || !strings.Contains(text, "Week 47   Mon 16 Nov to Sun 22 Nov 2026") || !strings.Contains(text, "Focused hours") {
		t.Fatalf("tab should show the week:\n%s", text)
	}
	w.run(w.press(key("3")))
	if text := plain(w); !strings.Contains(text, "November 2026") || !strings.Contains(text, "Focus by day") {
		t.Fatalf("3 should show the month:\n%s", text)
	}
	w.run(w.press(key("h")))
	if text := plain(w); !strings.Contains(text, "October 2026") {
		t.Fatalf("h should show the month before:\n%s", text)
	}
	w.run(w.press(key("t")))
	if text := plain(w); !strings.Contains(text, "November 2026") {
		t.Fatalf("t should come back to this month:\n%s", text)
	}
	w.run(w.press(key("1")))
	if text := plain(w); !strings.Contains(text, "Tuesday 17 November 2026") {
		t.Fatalf("1 should show the day again:\n%s", text)
	}
}

// Every row of every review is exactly as wide as the window, at the
// smallest size and a large one.
func TestEveryReviewRowFillsTheWindow(t *testing.T) {
	w := openedReview(t)
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		w.m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, period := range []string{"1", "2", "3"} {
			w.run(w.press(key(period)))
			for i, row := range screen(w) {
				if got := ansi.StringWidth(row); got != size[0] {
					t.Fatalf("%v %s: row %d is %d wide: %q", size, period, i, got, row)
				}
			}
		}
	}
}

func TestTheReviewScrollsWithinItsLength(t *testing.T) {
	w := openedReview(t)
	w.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	w.run(w.press(key("3")))
	w.press(key("k"))
	if w.m.reviewOffset != 0 {
		t.Fatal("scrolling above the top must do nothing")
	}
	for i := 0; i < 100; i++ {
		w.press(key("j"))
	}
	if text := plain(w); !strings.Contains(text, "6h or more") || strings.Contains(text, "[Month]") {
		t.Fatalf("scrolling down should reach the end and stop there:\n%s", text)
	}
}

func TestTheReviewChangesNothing(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	w.refresh()
	w.run(w.press(key("r")))
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

func TestAnEmptyDaySaysSo(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.run(w.press(key("r")))
	if text := plain(w); !strings.Contains(text, "Nothing was planned on this day.") {
		t.Fatalf("got:\n%s", text)
	}
}

// Help keeps its sections apart with two empty rows.
func TestHelpSectionsAreSeparatedByTwoEmptyRows(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("?"))
	rows := w.m.helpRows(78)
	var headings []int
	for i := range rows {
		if i+1 < len(rows) && strings.Contains(ansi.Strip(rows[i+1]), "────") {
			headings = append(headings, i)
		}
	}
	if len(headings) < 3 || headings[0] != 1 || strings.TrimSpace(rows[0]) != "" {
		t.Fatalf("the first section should follow one empty row: %v", headings)
	}
	for _, h := range headings[1:] {
		if strings.TrimSpace(rows[h-1]) != "" || strings.TrimSpace(rows[h-2]) != "" || strings.TrimSpace(ansi.Strip(rows[h-3])) == "" {
			t.Fatalf("section at row %d does not have exactly two empty rows above it", h)
		}
	}
}
