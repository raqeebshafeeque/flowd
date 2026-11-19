package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// leftoverDays plans the 15th and 16th and comes back to an empty 17th: on
// the 15th an item left incomplete and one skipped, on the 16th one left
// incomplete.
func leftoverDays(t *testing.T) *workspace {
	w := newWorkspace(t)
	w.now = time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	w.add("Write release notes", "1h")
	skipped := w.add("Reply to the review", "30m")
	w.act(planner.Skip, skipped)
	w.now = time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	w.add("Refactor the scheduler", "2h")
	w.now = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	w.run(w.m.read())
	return w
}

func TestThePanelCarriesWhatIsTicked(t *testing.T) {
	w := leftoverDays(t)
	w.run(w.press(key("C")))
	text := plain(w)
	for _, want := range []string{"Carry to today", "> [ ] Refactor the scheduler", "[ ] Write release notes", "Mon 16 Nov", "Sun 15 Nov", "INCOMPLETE", "SKIPPED", "carry 1 [enter]"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	w.press(space(), key("j"), key("j"), space())
	if text := plain(w); !strings.Contains(text, "carry 2 [enter]") || !strings.Contains(text, "[x] Reply to the review") {
		t.Fatalf("got:\n%s", text)
	}
	w.run(w.press(key("enter")))
	if w.m.carrying != nil {
		t.Fatal("the panel should close after carrying")
	}
	items := w.m.state.View.Items
	if len(items) != 2 || items[0].Title != "Refactor the scheduler" || items[1].Title != "Reply to the review" {
		t.Fatalf("got %+v", items)
	}
}

func TestAllTicksEverythingAndEscCarriesNothing(t *testing.T) {
	w := leftoverDays(t)
	w.run(w.press(key("C")))
	w.press(key("a"))
	if text := plain(w); !strings.Contains(text, "carry 3 [enter]") {
		t.Fatalf("got:\n%s", text)
	}
	w.press(key("a"))
	if text := plain(w); !strings.Contains(text, "carry 1 [enter]") {
		t.Fatalf("a again should clear them:\n%s", text)
	}
	w.press(key("esc"))
	w.run(w.m.read())
	if w.m.carrying != nil || len(w.m.state.View.Items) != 0 {
		t.Fatal("esc should carry nothing")
	}
}

func TestNothingToCarrySaysSo(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.run(w.press(key("C")))
	if text := plain(w); !strings.Contains(text, "Nothing to carry from the last 7 days.") {
		t.Fatalf("got:\n%s", text)
	}
}

// A past day cannot be carried onto, so C does nothing there.
func TestAPastDayCannotCarry(t *testing.T) {
	w := leftoverDays(t)
	w.m.date = "2026-11-16"
	w.run(w.m.read())
	if cmd := w.press(key("C")); cmd != nil || w.m.carrying != nil {
		t.Fatal("C should do nothing on a past day")
	}
}

// Every row stays exactly as wide as the window with the panel open.
func TestThePanelFillsTheWindow(t *testing.T) {
	w := leftoverDays(t)
	w.run(w.press(key("C")))
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		w.m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for i, row := range screen(w) {
			if got := ansi.StringWidth(row); got != size[0] {
				t.Fatalf("%v: row %d is %d wide: %q", size, i, got, row)
			}
		}
	}
}
