package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func TestHelpListsTheKeys(t *testing.T) {
	w := newWorkspace(t)
	w.add("Study", "1h")
	w.refresh()
	w.m.Update(tea.WindowSizeMsg{Width: 100, Height: 60})
	w.press(key("?"))
	if !w.m.help {
		t.Fatal("? should open help")
	}
	text := plain(w)
	for _, want := range []string{"Moving around", "Recording work (today only)", "Planning (today and later)", "In a form", "In the calendar", "Start, pause or resume", "Delete the selected item, after asking", "Enter never saves"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(text, "Last error") {
		t.Error("there is no error to show yet")
	}
	w.press(key("?"))
	if w.m.help {
		t.Fatal("? should close help")
	}
}

func TestHelpKeepsTheLastErrorAfterThePopupGoes(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.m.Update(tea.WindowSizeMsg{Width: 100, Height: 60})
	w.m.popup(domain.ErrDayClosed)
	w.now = w.now.Add(time.Minute)
	if w.m.visibleNotice() != nil {
		t.Fatal("the popup should have gone")
	}
	w.press(key("?"))
	text := plain(w)
	for _, want := range []string{"Last error", "That day is not open", "Only today's items can be started"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "ERR_") {
		t.Error("codes are for scripts, not for people")
	}
}

func TestHelpScrollsAndChangesNothing(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	w.refresh()
	w.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	w.press(key("?"))
	for i := 0; i < 50; i++ {
		w.press(key("j"))
	}
	w.m.View()
	if !strings.Contains(plain(w), "Show the chosen day") {
		t.Fatal("scrolling should reach the end of help")
	}
	for i, row := range screen(w) {
		if ansi.StringWidth(row) != 80 {
			t.Fatalf("row %d is %d wide", i, ansi.StringWidth(row))
		}
	}
	for _, k := range []tea.KeyPressMsg{space(), key("d"), key("n")} {
		if w.press(k) != nil {
			t.Fatal("keys acted inside help")
		}
	}
	if w.status(id) != "todo" || w.m.form != nil || w.m.deleting != "" {
		t.Fatal("help changed something")
	}
	w.press(key("esc"))
	if w.m.help {
		t.Fatal("esc should close help")
	}
}

func TestAQuestionMarkInAFormIsText(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("n"))
	w.typeText("Why?")
	if w.m.help || w.m.form.fields[0].value() != "Why?" {
		t.Fatal("? must be typed in a form")
	}
}
