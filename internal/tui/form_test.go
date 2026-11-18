package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func (w *workspace) typeText(text string) {
	for _, r := range text {
		w.press(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func ctrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

func tab() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyTab} }

func (w *workspace) item(id domain.ID) domain.Item {
	for _, item := range w.m.state.View.Items {
		if item.ID == id {
			return item.Item
		}
	}
	return domain.Item{}
}

func TestNewItemFromTheWorkspace(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("n"))
	if w.m.form == nil || w.m.form.edit {
		t.Fatal("n should open a new item form")
	}
	w.typeText("Read the Go spec")
	w.press(key("enter")) // moves to Duration, never saves
	if w.m.form == nil || w.m.form.focus != 1 {
		t.Fatal("enter should move to the Duration field")
	}
	w.typeText("1h30m")
	w.run(w.press(ctrl('s')))
	if w.m.form != nil || len(w.m.state.View.Items) != 1 {
		t.Fatal("ctrl+s should save and close the form")
	}
	got := w.m.state.View.Items[0]
	if got.Title != "Read the Go spec" || got.DurationMinutes != 90 || w.m.selected != got.ID {
		t.Fatalf("unexpected item: %+v", got)
	}
}

func TestLettersAreTextWhileTyping(t *testing.T) {
	w := newWorkspace(t)
	w.add("Study", "1h")
	w.refresh()
	w.press(key("n"))
	w.typeText("q s f n e ?")
	if w.m.form == nil || w.m.form.fields[0].value() != "q s f n e ?" {
		t.Fatal("letters must be typed, not run as actions")
	}
	if w.status(w.m.state.View.Items[0].ID) != domain.Todo {
		t.Fatal("typing changed an item")
	}
}

func TestEditChangesOnlyWhatChanged(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	w.refresh()
	w.press(key("e"))
	if w.m.form == nil || w.m.form.fields[0].value() != "Study" || w.m.form.fields[1].value() != "1h" {
		t.Fatal("edit should start from the saved values")
	}
	w.press(tab())
	w.press(key("backspace"), key("backspace"))
	w.typeText("45m")
	w.run(w.press(ctrl('s')))
	if got := w.item(id); got.Title != "Study" || got.DurationMinutes != 45 {
		t.Fatalf("unexpected item: %+v", got)
	}
}

func TestSavingAnUnchangedEditJustCloses(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	w.refresh()
	w.press(key("e"))
	if w.press(ctrl('s')) != nil || w.m.form != nil {
		t.Fatal("an unchanged edit should close without saving")
	}
	if events := w.m.state.History[id]; len(events) != 1 {
		t.Fatal("an unchanged edit wrote an event")
	}
}

func TestABadDurationIsExplainedUnderTheField(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("n"))
	w.typeText("Study")
	w.press(tab())
	w.typeText("1.5h")
	w.run(w.press(ctrl('s')))
	if w.m.form == nil || w.m.form.focus != 1 || !strings.Contains(w.m.form.errors[1], "Invalid duration") {
		t.Fatalf("the error should sit under Duration: %+v", w.m.form)
	}
	if w.m.form.fields[0].value() != "Study" || w.m.form.fields[1].value() != "1.5h" {
		t.Fatal("what was typed must be kept")
	}
	if w.m.visibleNotice() != nil {
		t.Fatal("a field error should not also pop up")
	}
	if text := plain(w); !strings.Contains(text, "Invalid duration") {
		t.Fatalf("the error is not on screen:\n%s", text)
	}
	if len(w.m.state.View.Items) != 0 {
		t.Fatal("nothing should be saved")
	}
}

func TestEscAsksBeforeThrowingAwayChanges(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("n"), key("esc"))
	if w.m.form != nil {
		t.Fatal("an empty form should close right away")
	}
	w.press(key("n"))
	w.typeText("Study")
	w.press(key("esc"))
	if w.m.form == nil || !w.m.form.discard || !strings.Contains(plain(w), "Discard changes?") {
		t.Fatal("esc should ask first")
	}
	w.press(key("n"))
	if w.m.form == nil || w.m.form.discard || w.m.form.fields[0].value() != "Study" {
		t.Fatal("no should keep the form and the text")
	}
	w.press(key("esc"), key("y"))
	if w.m.form != nil {
		t.Fatal("yes should throw the changes away")
	}
}

func TestCaretEditingAndPaste(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("n"))
	w.typeText("Stdy")
	w.press(key("left"), key("left"), key("left"))
	w.typeText("u")
	w.m.Update(tea.PasteMsg{Content: "\nnotes"})
	if got := w.m.form.fields[0].value(); got != "Su notestdy" {
		t.Fatalf("got %q", got)
	}
	w.press(key("home"))
	w.press(key("delete"))
	if got := w.m.form.fields[0].value(); got != "u notestdy" {
		t.Fatalf("got %q", got)
	}
}

func TestTheCaretIsPlacedInTheFocusedField(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("n"))
	w.typeText("Study")
	v := w.m.View()
	if v.Cursor == nil {
		t.Fatal("the form should show a caret")
	}
	rows := strings.Split(ansi.Strip(v.Content), "\n")
	row := rows[v.Cursor.Y]
	before := ansi.Cut(row, 0, v.Cursor.X)
	if !strings.Contains(row, "Title") || !strings.HasSuffix(before, "Study") {
		t.Fatalf("caret at %d,%d in %q", v.Cursor.X, v.Cursor.Y, row)
	}
	w.press(key("esc"), key("y"))
	if w.m.View().Cursor != nil {
		t.Fatal("no caret without a form")
	}
}

func TestTheDurationHintShowsWhileEmpty(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("n"))
	if !strings.Contains(plain(w), "e.g. 30m, 1h, 1h30m") {
		t.Fatal("the empty Duration should show an example")
	}
}

func TestAHiddenCharacterIsNamedUnderTheTitle(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("n"))
	w.m.Update(tea.PasteMsg{Content: "Study\u202e"})
	w.press(tab())
	w.typeText("1h")
	w.run(w.press(ctrl('s')))
	if w.m.form == nil || w.m.form.focus != 0 || !strings.Contains(w.m.form.errors[0], "U+202E") {
		t.Fatalf("the title error should name the character: %+v", w.m.form)
	}
}

func TestTooLongADurationIsExplainedAtTheField(t *testing.T) {
	w := newWorkspace(t) // 09:00, fifteen hours left
	w.refresh()
	w.press(key("n"))
	w.typeText("Too much")
	w.press(tab())
	w.typeText("30h")
	w.run(w.press(ctrl('s')))
	if w.m.form == nil || w.m.form.focus != 1 || !strings.Contains(w.m.form.errors[1], "Only 15h left on this day") {
		t.Fatalf("got %+v", w.m.form)
	}
	if w.m.form.fields[1].value() != "30h" || len(w.m.state.View.Items) != 0 {
		t.Fatal("the typed value must stay and nothing may be saved")
	}
}

func TestOneEmptyRowSeparatesTheFields(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.press(key("n"))
	for _, withError := range []bool{false, true} {
		if withError {
			w.m.form.errors[0] = "Check title"
		}
		rows, _, _ := w.m.formRows(78, 12)
		var title, duration int
		for i, row := range rows {
			text := ansi.Strip(row)
			if strings.HasPrefix(text, " Title") {
				title = i
			}
			if strings.HasPrefix(text, " Duration") {
				duration = i
			}
		}
		gap := 1
		if withError {
			gap = 2 // the error line sits right under the title
		}
		if duration-title != gap+1 || strings.TrimSpace(rows[duration-1]) != "" {
			t.Fatalf("error=%v: title at %d, duration at %d", withError, title, duration)
		}
	}
}
