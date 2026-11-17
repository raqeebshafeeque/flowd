package tui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// workspace is a model on a fresh database with a clock the test controls.
type workspace struct {
	t      *testing.T
	m      *model
	client planner.Client
	now    time.Time
}

func newWorkspace(t *testing.T) *workspace {
	w := &workspace{t: t, now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
	clock := func() time.Time { return w.now }
	w.client = planner.Client{Path: filepath.Join(t.TempDir(), "flowd.sqlite"), Now: clock, Zone: time.UTC}
	w.m = newModel(context.Background(), w.client, time.UTC, clock, true)
	w.m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return w
}

// add plans an item through the planner, the way the CLI would.
func (w *workspace) add(title, duration string) domain.ID {
	w.t.Helper()
	r, err := w.client.Do(context.Background(), planner.Intent{Operation: planner.Add, Title: &title, Duration: &duration})
	if err != nil {
		w.t.Fatal(err)
	}
	return r.Item.ID
}

func (w *workspace) act(op planner.Operation, id domain.ID) {
	w.t.Helper()
	state, err := w.client.Read(context.Background(), planner.Selection{})
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.client.Do(context.Background(), planner.Intent{Operation: op, ItemID: id, Revision: state.Revisions[id]}); err != nil {
		w.t.Fatal(err)
	}
}

// refresh runs one background read and hands the result to the model.
func (w *workspace) refresh() {
	w.m.Update(w.m.read()())
}

func (w *workspace) press(keys ...tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		_, cmd = w.m.Update(k)
	}
	return cmd
}

func key(text string) tea.KeyPressMsg {
	switch text {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(text[0]), Text: text}
}

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestSelectionMovesWithinTheList(t *testing.T) {
	w := newWorkspace(t)
	a, b := w.add("A", "1h"), w.add("B", "1h")
	w.refresh()
	if w.m.selected != a {
		t.Fatal("the first item should be selected when the day loads")
	}
	w.press(key("j"))
	if w.m.selected != b {
		t.Fatal("j should select the next item")
	}
	w.press(key("down"), key("j"), key("j"))
	if w.m.selected != b {
		t.Fatal("moving past the end should stay on the last item")
	}
	w.press(key("k"), key("k"))
	if w.m.selected != a {
		t.Fatal("moving past the start should stay on the first item")
	}
}

func TestSelectionFollowsTheItemWhenTheOrderChanges(t *testing.T) {
	w := newWorkspace(t)
	a, b := w.add("A", "1h"), w.add("B", "1h")
	w.refresh()
	w.press(key("j")) // select B
	w.act(planner.Start, b)     // B moves to the top
	w.refresh()
	if w.m.selected != b || w.m.index() != 0 {
		t.Fatalf("selection lost B: %v at %d", w.m.selected, w.m.index())
	}
	_ = a
}

func TestSelectionStaysInPlaceWhenTheItemDisappears(t *testing.T) {
	w := newWorkspace(t)
	w.add("A", "1h")
	b := w.add("B", "1h")
	c := w.add("C", "1h")
	w.refresh()
	w.press(key("j")) // select B
	w.act(planner.Delete, b)
	w.refresh()
	if w.m.selected != c {
		t.Fatal("the next item should be selected after B was deleted")
	}
}

func TestDetailsOpenAndClose(t *testing.T) {
	w := newWorkspace(t)
	w.press(key("enter"))
	if w.m.details {
		t.Fatal("details cannot open without a selected item")
	}
	w.add("Study", "1h")
	w.refresh()
	w.press(key("enter"))
	if !w.m.details {
		t.Fatal("enter should open details")
	}
	w.press(key("esc"))
	if w.m.details {
		t.Fatal("esc should close details")
	}
	w.press(key("v"), key("v"))
	if w.m.details {
		t.Fatal("v should toggle details")
	}
}

func TestQuit(t *testing.T) {
	w := newWorkspace(t)
	if !quits(w.press(key("q"))) || !quits(w.press(key("ctrl+c"))) {
		t.Fatal("q and ctrl+c should quit")
	}
}

func TestASmallWindowIgnoresEveryKeyButCtrlC(t *testing.T) {
	w := newWorkspace(t)
	w.add("Study", "1h")
	w.refresh()
	w.m.Update(tea.WindowSizeMsg{Width: 79, Height: 30})
	if quits(w.press(key("q"))) {
		t.Fatal("q must do nothing while the window is too small")
	}
	before := w.m.selected
	w.press(key("j"), key("enter"))
	if w.m.selected != before || w.m.details {
		t.Fatal("keys changed the hidden workspace")
	}
	if !quits(w.press(key("ctrl+c"))) {
		t.Fatal("ctrl+c must always quit")
	}
}

func TestTicksRefreshFromStorage(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.add("Added elsewhere", "1h")
	_, cmd := w.m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("a tick must schedule a read and the next tick")
	}
	w.refresh()
	if len(w.m.state.View.Items) != 1 {
		t.Fatal("an item added by another process was not picked up")
	}
}

func TestAFailedReadKeepsTheLastGoodDay(t *testing.T) {
	w := newWorkspace(t)
	w.add("Study", "1h")
	w.refresh()
	w.m.Update(stateMsg{err: context.DeadlineExceeded})
	if w.m.err == nil || len(w.m.state.View.Items) != 1 {
		t.Fatal("the last good day must stay visible with the error")
	}
	w.refresh()
	if w.m.err != nil {
		t.Fatal("a good read must clear the error")
	}
}
