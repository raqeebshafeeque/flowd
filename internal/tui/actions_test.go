package tui

import (
	"errors"
	"strings"
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

// Space starts, pauses, resumes and reopens, and f finishes.
func TestSpaceAndFTrackTheSelectedItem(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	w.refresh()
	steps := []struct {
		key  tea.KeyPressMsg
		want domain.Status
	}{{space(), domain.InProgress}, {space(), domain.Paused}, {space(), domain.InProgress}, {key("f"), domain.Done}, {space(), domain.InProgress}}
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
	if w.press(key("f")) != nil {
		t.Fatal("a second change was sent while the first was saving")
	}
	w.run(first)
	if w.m.pending {
		t.Fatal("saving never finished")
	}
}

// When a save may or may not have landed, the workspace says so and the next
// read shows what is really saved.
func TestAnUnconfirmedChangeIsShownAndSettledByTheNextRead(t *testing.T) {
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
	if w.m.visibleNotice() == nil {
		t.Fatal("an unknown outcome must be shown")
	}
	w.run(cmd)
	w.refresh()
	if w.status(id) != domain.InProgress {
		t.Fatalf("the next read should show the start: %s", w.status(id))
	}
}

func TestDeleteAsksFirstAndOnlyYesDeletes(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	w.refresh()
	w.press(key("d"))
	if w.m.deleting != id || !strings.Contains(plain(w), `Delete "Study"?  yes [y]  no [n]`) {
		t.Fatalf("d should ask in the footer:\n%s", plain(w))
	}
	for _, k := range []tea.KeyPressMsg{key("j"), space(), key("x")} {
		if w.press(k) != nil || w.m.deleting != id {
			t.Fatal("only y, n or esc may answer the question")
		}
	}
	w.press(key("n"))
	if w.m.deleting != "" || len(w.m.state.View.Items) != 1 {
		t.Fatal("n should cancel")
	}
	w.press(key("d"), key("esc"))
	if w.m.deleting != "" {
		t.Fatal("esc should cancel")
	}
	w.press(key("d"))
	w.run(w.press(key("y")))
	if len(w.m.state.View.Items) != 0 {
		t.Fatal("y should delete")
	}
	if events, err := w.client.Read(w.m.ctx, planner.Selection{}); err != nil || len(events.View.Items) != 0 {
		t.Fatal("the item should be hidden from the day", err)
	}
}

func TestPastItemsCannotBeDeleted(t *testing.T) {
	w := newWorkspace(t)
	w.add("Yesterday", "1h")
	w.now = w.now.Add(24 * time.Hour)
	w.refresh()
	w.press(key("c"), key("h"))
	w.run(w.press(key("enter")))
	w.press(key("d"))
	if w.m.deleting != "" {
		t.Fatal("a past day's item must not be deletable")
	}
}

func TestALongTitleStillLeavesRoomForTheAnswers(t *testing.T) {
	w := newWorkspace(t)
	w.add(strings.Repeat("Very long title ", 7), "1h")
	w.refresh()
	w.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	w.press(key("d"))
	if text := plain(w); !strings.Contains(text, "…\"?  yes [y]  no [n]") {
		t.Fatalf("the answers were cut off:\n%s", text)
	}
}
