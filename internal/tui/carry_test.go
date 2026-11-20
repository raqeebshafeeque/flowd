package tui

import (
	"strings"
	"testing"
	"time"
)

// leftoverDays plans the 15th and 16th and comes back to an empty 17th: two
// items left incomplete on the 15th and one on the 16th.
func leftoverDays(t *testing.T) *workspace {
	w := newWorkspace(t)
	w.now = time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	w.add("Write release notes", "1h")
	w.add("Reply to the review", "30m")
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
	for _, want := range []string{"Carry to today", "> [ ] Refactor the scheduler", "[ ] Write release notes", "Mon 16 Nov", "Sun 15 Nov", "INCOMPLETE", "pick [space]"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	w.press(space(), key("j"), key("j"), space())
	if text := plain(w); !strings.Contains(text, "carry 2 [ctrl+s]") || !strings.Contains(text, "[x] Reply to the review") {
		t.Fatalf("got:\n%s", text)
	}
	if cmd := w.press(key("enter")); cmd != nil || w.m.carrying == nil {
		t.Fatal("enter should do nothing: only ctrl+s saves")
	}
	w.run(w.press(ctrl('s')))
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
	if text := plain(w); !strings.Contains(text, "carry 3 [ctrl+s]") {
		t.Fatalf("got:\n%s", text)
	}
	w.press(key("a"))
	if text := plain(w); strings.Contains(text, "[ctrl+s]") {
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
