package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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
		{key("l"), "2026-11-18"}, {key("t"), "2026-11-17"},
	}
	for _, s := range steps {
		w.press(s.key)
		if got := w.m.calendar.cursor.Format("2006-01-02"); got != s.want {
			t.Fatalf("after %s got %s, want %s", s.key.String(), got, s.want)
		}
	}
}

func TestTheCalendarShowsTheMonthWithTheCursorBracketed(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("c"))
	text := plain(w)
	for _, want := range []string{"Calendar", "November 2026", "Mo  Tu  We  Th  Fr  Sa  Su", "[17]", "show [enter]", "month [m]", "year [y]"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	w.m.style.noColor = false
	if !strings.Contains(w.m.View().Content, "\x1b[36m[17]") {
		t.Error("the cursor date should be teal")
	}
}

func TestMonthAndYearCanBeTyped(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("c"))
	// Move to 31 January first, so the jump has to keep the day valid.
	w.m.calendar.cursor = time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)
	w.press(key("m"))
	if w.m.calendar.input == nil || w.m.calendar.input.value() != "1" || !strings.Contains(plain(w), "Month (1-12): 1") {
		t.Fatalf("m should open the month input with the current month:\n%s", plain(w))
	}
	if v := w.m.View(); v.Cursor == nil || v.Cursor.Y != w.m.height-2 || !strings.Contains(strings.Split(plain(w), "\n")[v.Cursor.Y], "Month (1-12)") {
		t.Fatal("the caret should be in the footer")
	}
	w.press(key("backspace"))
	w.typeText("2")
	w.press(key("enter"))
	if got := w.m.calendar.cursor.Format("2006-01-02"); got != "2027-02-28" {
		t.Fatalf("got %s, want the last day of February", got)
	}
	w.press(key("y"))
	w.press(key("backspace"), key("backspace"), key("backspace"), key("backspace"))
	w.typeText("2028")
	w.press(key("enter"))
	if got := w.m.calendar.cursor.Format("2006-01-02"); got != "2028-02-28" {
		t.Fatalf("got %s", got)
	}
}

func TestATypedMonthOrYearOutOfRangeChangesNothing(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("c"))
	before := w.m.calendar.cursor
	w.press(key("m"), key("backspace"), key("backspace"))
	w.typeText("13")
	w.press(key("enter"))
	if w.m.calendar.input == nil || w.m.calendar.cursor != before || !strings.Contains(plain(w), "Enter a month from 1 to 12") {
		t.Fatal("13 must stay in the input with an explanation")
	}
	w.press(key("esc"))
	if w.m.calendar == nil || w.m.calendar.input != nil || w.m.calendar.cursor != before {
		t.Fatal("esc should close the input and keep the calendar where it was")
	}
	w.press(key("y"))
	w.typeText("x")
	w.press(key("enter"))
	if !strings.Contains(plain(w), "Enter a year from 1 to 9999") {
		t.Fatal("a non-number year must be explained")
	}
	w.press(key("esc"), key("j"))
	if w.m.calendar.cursor == before {
		t.Fatal("after the input closes, keys move the calendar again")
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
	if text := plain(w); !strings.Contains(text, "Viewing: 2026-11-18") || !strings.Contains(text, "today [t]") {
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
	w.press(key("a"))
	w.typeText("Tomorrow's item")
	w.press(tab())
	w.typeText("1h")
	w.run(w.press(key("enter")))
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
	w.press(key("a"))
	w.press(key("e"))
	if w.m.form != nil {
		t.Fatal("a past day must not open a form")
	}
	text := plain(w)
	if strings.Contains(text, "add [a]") || strings.Contains(text, "edit [e]") || strings.Contains(text, "Press n") {
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
