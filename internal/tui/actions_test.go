package tui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

// run performs the command a key returned and feeds every resulting message
// back to the model, the way the program loop would.
func (w *workspace) run(cmd tea.Cmd) {
	w.t.Helper()
	for cmd != nil {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				w.run(c)
			}
			return
		}
		_, cmd = w.m.Update(msg)
	}
}

func (w *workspace) status(id domain.ID) domain.Status {
	for _, item := range w.m.state.View.Items {
		if item.ID == id {
			return item.Status
		}
	}
	return ""
}

func space() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "} }

func TestSpaceStartsPausesAndResumes(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	w.refresh()
	for _, want := range []domain.Status{domain.InProgress, domain.Paused, domain.InProgress} {
		w.now = w.now.Add(time.Minute)
		w.run(w.press(space()))
		if got := w.status(id); got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	}
}

func TestLetterKeysFinishSkipUnskipAndReopen(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	w.refresh()
	steps := []struct {
		key  tea.KeyPressMsg
		want domain.Status
	}{{key("s"), domain.Skipped}, {key("u"), domain.Todo}, {space(), domain.InProgress}, {key("f"), domain.Done}, {key("R"), domain.InProgress}}
	for _, s := range steps {
		w.now = w.now.Add(time.Minute)
		w.run(w.press(s.key))
		if got := w.status(id); got != s.want {
			t.Fatalf("%s gave %s, want %s", s.key.String(), got, s.want)
		}
	}
}

func TestStartingAnotherItemPausesTheRunningOneQuietly(t *testing.T) {
	w := newWorkspace(t)
	a := w.add("A", "1h")
	b := w.add("B", "1h")
	w.refresh()
	w.run(w.press(space()))
	w.now = w.now.Add(time.Minute)
	w.press(key("j")) // B is second now that A started
	w.run(w.press(space()))
	if w.status(a) != domain.Paused || w.status(b) != domain.InProgress {
		t.Fatalf("A %s, B %s", w.status(a), w.status(b))
	}
	if w.m.visibleNotice() != nil {
		t.Fatal("a successful action must not pop up anything")
	}
}

func TestARefusalPopsUpAndChangesNothing(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	w.refresh()
	w.run(w.press(key("f"))) // a todo item cannot be finished
	n := w.m.visibleNotice()
	if n == nil || n.feedback.Code != "ERR_ILLEGAL_TRANSITION" {
		t.Fatalf("expected a refusal popup, got %+v", n)
	}
	if w.status(id) != domain.Todo {
		t.Fatal("the refused action changed the item")
	}
}

func TestNoSecondChangeWhileOneIsSaving(t *testing.T) {
	w := newWorkspace(t)
	w.add("Study", "1h")
	w.refresh()
	first := w.press(space())
	if first == nil || !w.m.pending {
		t.Fatal("space should start saving")
	}
	if w.press(key("s")) != nil {
		t.Fatal("a second change was sent while the first was saving")
	}
	w.run(first)
	if w.m.pending {
		t.Fatal("saving never finished")
	}
}

func TestAnUnconfirmedChangeIsCheckedBeforeAnythingElse(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	w.refresh()
	// Start for real, then pretend the confirmation got lost.
	state := w.m.state
	r, err := w.client.Do(w.m.ctx, planner.Intent{Operation: planner.Start, ItemID: id, Revision: state.Revisions[id]})
	if err != nil {
		t.Fatal(err)
	}
	r.Outcome = planner.Unconfirmed
	cmd := w.m.finished(actionMsg{result: r, err: errors.New("disk vanished")})
	if w.m.unresolved == nil || w.m.visibleNotice() == nil {
		t.Fatal("an unknown outcome must be shown and remembered")
	}
	if w.press(key("s")) != nil {
		t.Fatal("no other change may start while the outcome is unknown")
	}
	w.run(cmd)
	if w.m.unresolved != nil || w.status(id) != domain.InProgress {
		t.Fatalf("reconcile should prove the start happened: %v %s", w.m.unresolved, w.status(id))
	}
}
