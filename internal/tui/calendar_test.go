package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

func TestTheCalendarMovesItsCursor(t *testing.T) {
	w := newWorkspace(t) // Tuesday 17 November 2026
	w.refresh()
	w.press(key("c"))
	if w.m.calendar == nil {
		t.Fatal("c should open the calendar")
	}
	steps := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{key("l"), "2026-11-18"}, {key("h"), "2026-11-17"},
		{key("j"), "2026-11-24"}, {key("k"), "2026-11-17"},
		{key(">"), "2026-12-17"}, {key("<"), "2026-11-17"},
		{key("<"), "2026-10-17"}, {key("t"), "2026-11-17"},
	}
	for _, s := range steps {
		w.press(s.key)
		if got := w.m.calendar.cursor.Format("2006-01-02"); got != s.want {
			t.Fatalf("after %s got %s, want %s", s.key.String(), got, s.want)
		}
	}
}

func TestMonthsKeepTheDayWhereTheyCan(t *testing.T) {
	jan31 := time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)
	if got := addMonths(jan31, 1).Format("2006-01-02"); got != "2027-02-28" {
		t.Fatalf("got %s", got)
	}
	if got := addMonths(jan31, -2).Format("2006-01-02"); got != "2026-11-30" {
		t.Fatalf("got %s", got)
	}
}

func TestTheCalendarDrawsTheMonthWithTheCursorBracketed(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("c"))
	text := plain(w)
	for _, want := range []string{"Calendar", "November 2026", "Mo  Tu  We  Th  Fr  Sa  Su", "[17]", "Selected Tue 17 Nov 2026 (today)", "show [enter]"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	// November 2026 starts on a Sunday, so the first week has only the 1st.
	if !strings.Contains(text, "                           1 ") {
		t.Errorf("the 1st is not under Sunday:\n%s", text)
	}
}

func TestChoosingADayShowsItWithoutCreatingIt(t *testing.T) {
	w := newWorkspace(t)
	w.add("Today's item", "1h")
	w.refresh()
	w.press(key("c"), key("l"))
	w.run(w.press(key("enter")))
	if w.m.calendar != nil || w.m.date != "2026-11-18" || len(w.m.state.View.Items) != 0 {
		t.Fatalf("not showing an empty 18 November: %q %d", w.m.date, len(w.m.state.View.Items))
	}
	if text := plain(w); !strings.Contains(text, "Viewing 2026-11-18") || !strings.Contains(text, "today [t]") {
		t.Fatalf("the header should say which day is shown:\n%s", text)
	}
	if state, _ := w.client.Read(w.m.ctx, planner.Selection{Date: "2026-11-18"}); state.Day.ID != "" {
		t.Fatal("looking at a day must not create it")
	}
	w.run(w.press(key("t")))
	if w.m.date != "" || len(w.m.state.View.Items) != 1 {
		t.Fatal("t should come back to today")
	}
}

func TestFutureDaysCanBePlannedButNotTracked(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("c"), key("l"))
	w.run(w.press(key("enter")))
	w.press(key("n"))
	w.typeText("Tomorrow's item")
	w.press(tab())
	w.typeText("1h")
	w.run(w.press(ctrl('s')))
	if len(w.m.state.View.Items) != 1 || w.m.state.Day.Date != "2026-11-18" {
		t.Fatalf("the item should be planned on the 18th: %+v", w.m.state.Day)
	}
	if w.press(space()) != nil {
		t.Fatal("tomorrow's item must not start today")
	}
	if strings.Contains(plain(w), "start [space]") {
		t.Fatal("the footer must not offer to start tomorrow's item")
	}
}

func TestPastDaysCanOnlyBeLookedAt(t *testing.T) {
	w := newWorkspace(t)
	w.add("Yesterday's work", "1h")
	w.now = w.now.Add(24 * time.Hour) // it is now the 18th
	w.refresh()
	w.press(key("c"), key("h"))
	w.run(w.press(key("enter")))
	if len(w.m.state.View.Items) != 1 || w.m.state.View.Items[0].Status != "incomplete" {
		t.Fatalf("the 17th should show its closed item: %+v", w.m.state.View.Items)
	}
	w.press(key("n"))
	w.press(key("e"))
	if w.m.form != nil {
		t.Fatal("a past day must not open a form")
	}
	text := plain(w)
	if strings.Contains(text, "new [n]") || strings.Contains(text, "edit [e]") || strings.Contains(text, "Press n") {
		t.Fatalf("the footer offers planning on a past day:\n%s", text)
	}
}

func TestAnAnswerForAnotherDateIsIgnored(t *testing.T) {
	w := newWorkspace(t)
	w.add("Today's item", "1h")
	w.refresh()
	stale := w.m.read() // started while today was on screen
	w.press(key("c"), key("l"))
	w.run(w.press(key("enter")))
	w.m.Update(stale())
	if len(w.m.state.View.Items) != 0 {
		t.Fatal("a late answer for today replaced the 18th")
	}
}

func TestTheCalendarFitsTheSmallestWindow(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	w.press(key("c"))
	if text := plain(w); !strings.Contains(text, "Selected") {
		t.Fatalf("the calendar is cut off at 80x24:\n%s", text)
	}
	for i, row := range screen(w) {
		if ansi.StringWidth(row) != 80 {
			t.Fatalf("row %d is %d wide", i, ansi.StringWidth(row))
		}
	}
}
