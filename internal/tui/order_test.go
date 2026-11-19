package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

func (w *workspace) titles() string {
	var names []string
	for _, it := range w.m.state.View.Items {
		names = append(names, it.Title)
	}
	return strings.Join(names, " ")
}

func threeItems(t *testing.T) *workspace {
	w := newWorkspace(t)
	w.add("A", "30m")
	w.add("B", "30m")
	w.add("C", "30m")
	w.refresh()
	return w
}

func TestReorderingMovesOnScreenAndSavesWithCtrlS(t *testing.T) {
	w := threeItems(t)
	w.press(key("o"))
	text := plain(w)
	if w.m.ordering == nil || !strings.Contains(text, "Reordering") || !strings.Contains(text, "move [J/K]  select [j/k]  save [ctrl+s]  cancel [esc]") {
		t.Fatalf("got:\n%s", text)
	}
	w.press(key("J"), key("J"))
	if w.titles() != "B C A" || w.m.state.View.Items[2].ID != w.m.selected {
		t.Fatalf("A should move down and stay selected: %s", w.titles())
	}
	// Nothing is saved yet: reading the day still gives the old order.
	if state, _ := w.client.Read(w.m.ctx, planner.Selection{}); state.View.Items[0].Title != "A" {
		t.Fatal("moving must not save")
	}
	w.run(w.press(ctrl('s')))
	if w.m.ordering != nil || w.titles() != "B C A" {
		t.Fatalf("got %s", w.titles())
	}
	if state, _ := w.client.Read(w.m.ctx, planner.Selection{}); state.View.Items[0].Title != "B" {
		t.Fatal("ctrl+s should save the order")
	}
}

// Esc asks before throwing a new order away, and only y throws it away.
func TestCancellingAsksAfterAMove(t *testing.T) {
	w := threeItems(t)
	w.press(key("o"), key("esc"))
	if w.m.ordering != nil {
		t.Fatal("esc with nothing moved should just leave")
	}
	w.press(key("o"), key("J"), key("esc"))
	if !strings.Contains(plain(w), "Throw away the new order?  yes [y]  no [n]") {
		t.Fatalf("got:\n%s", plain(w))
	}
	w.press(key("n"))
	if w.m.ordering == nil || w.titles() != "B A C" {
		t.Fatal("n should keep reordering")
	}
	w.press(key("esc"), key("y"))
	if w.m.ordering != nil || w.titles() != "A B C" {
		t.Fatalf("y should put the order back: %s", w.titles())
	}
}

// While reordering, tracking keys do nothing and the day is not replaced.
func TestNothingElseHappensWhileReordering(t *testing.T) {
	w := threeItems(t)
	w.press(key("o"), key("J"))
	for _, k := range []tea.KeyPressMsg{space(), key("f"), key("n"), key("d"), key("C")} {
		if cmd := w.press(k); cmd != nil {
			t.Fatalf("%s did something while reordering", k.String())
		}
	}
	w.refresh()
	if w.titles() != "B A C" {
		t.Fatalf("a refresh replaced the order being arranged: %s", w.titles())
	}
}

func TestOnlyAPlannableDayWithTwoItemsCanBeReordered(t *testing.T) {
	w := newWorkspace(t)
	w.add("A", "30m")
	w.refresh()
	w.press(key("o"))
	if w.m.ordering != nil || strings.Contains(strings.Join(w.m.hints(), " "), "order [o]") {
		t.Fatal("one item has no order to change")
	}
	w.add("B", "30m")
	w.m.date = "2026-11-16"
	w.refresh()
	w.press(key("o"))
	if w.m.ordering != nil {
		t.Fatal("a past day cannot be reordered")
	}
}

func TestReorderingFillsTheWindow(t *testing.T) {
	w := threeItems(t)
	w.press(key("o"), key("J"), key("esc"))
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		w.m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for i, row := range screen(w) {
			if got := ansi.StringWidth(row); got != size[0] {
				t.Fatalf("%v: row %d is %d wide", size, i, got)
			}
		}
	}
}
