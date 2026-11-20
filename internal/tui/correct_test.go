package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// forgotten plans an item on the 16th, never finished, and opens that day
// from the 17th.
func forgotten(t *testing.T) (*workspace, domain.ID) {
	w := newWorkspace(t)
	w.now = time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	id := w.add("Forgot to finish", "1h")
	w.now = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	w.m.date = "2026-11-16"
	w.refresh()
	return w, id
}

func TestFOnAPastDayAsksAndMarksDone(t *testing.T) {
	w, id := forgotten(t)
	if !strings.Contains(plain(w), "mark done [f]") {
		t.Fatalf("got:\n%s", plain(w))
	}
	w.press(key("f"))
	if !strings.Contains(plain(w), `Mark "Forgot to finish" done? It was left incomplete.  yes [y]  no [n]`) {
		t.Fatalf("got:\n%s", plain(w))
	}
	w.press(key("n"))
	if w.status(id) != "incomplete" {
		t.Fatal("n should change nothing")
	}
	w.press(key("f"))
	w.run(w.press(key("y")))
	w.refresh()
	if w.status(id) != "done" {
		t.Fatalf("got %s", w.status(id))
	}
	w.press(key("enter"))
	if text := plain(w); !strings.Contains(text, "Marked done later, on Tue 17 Nov") {
		t.Fatalf("details should show the correction:\n%s", text)
	}
	w.press(key("esc"))
	if strings.Contains(plain(w), "mark done [f]") {
		t.Fatal("a done item cannot be marked done again")
	}
}

func TestTheQuestionFitsTheWindow(t *testing.T) {
	w, _ := forgotten(t)
	w.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	w.press(key("f"))
	for i, row := range screen(w) {
		if got := ansi.StringWidth(row); got != 80 {
			t.Fatalf("row %d is %d wide", i, got)
		}
	}
}

// More than seven days back, f does nothing.
func TestOnlyTheLastSevenDaysCanBePutRight(t *testing.T) {
	w := newWorkspace(t)
	w.now = time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	w.add("Too old", "1h")
	w.now = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	w.m.date = "2026-11-09"
	w.refresh()
	if cmd := w.press(key("f")); cmd != nil || w.m.correcting != "" || strings.Contains(plain(w), "mark done") {
		t.Fatal("too old to put right")
	}
}
