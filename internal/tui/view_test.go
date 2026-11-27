package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

func screen(w *workspace) []string {
	return strings.Split(w.m.View().Content, "\n")
}

func plain(w *workspace) string {
	return ansi.Strip(w.m.View().Content)
}

func TestTheListShowsWorkDurationAndStatus(t *testing.T) {
	w := newWorkspace(t)
	a := w.add("Study", "1h")
	w.add("Code", "30m")
	w.act(planner.Start, a)
	w.now = w.now.Add(10 * time.Minute)
	w.refresh()
	w.press(key(".")) // the rest of the keys, which do not fit on one row
	text := plain(w)
	for _, want := range []string{"flowd", "Tue 17 Nov 2026 │ 09:10:00", "> Study", "10m / 1h", "IN PROGRESS", "Code", "30m", "TODO", "add [a]  edit [e]  pause [space]  finish [f]  delete [d]", "help [?]", "quit [q]"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestDetailsShowEveryMeasurement(t *testing.T) {
	w := newWorkspace(t)
	a := w.add("Study the scheduler", "1h30m")
	w.act(planner.Start, a)
	w.now = w.now.Add(5*time.Minute + 6*time.Second)
	w.refresh()
	w.press(key("enter"))
	text := plain(w)
	for _, want := range []string{"Details", "Title        Study the scheduler", "Duration     1h30m", "Work         5m6s", "Paused       0m", "Started At   17 Nov 2026 09:00:00", "Finished At  -", "close [esc]"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestAnEmptyDaySaysHowToStart(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	if text := plain(w); !strings.Contains(text, "Nothing planned for this day.") || strings.Contains(text, "details [enter]") {
		t.Fatalf("got:\n%s", text)
	}
}

func TestASmallWindowOnlyAsksToBeLarger(t *testing.T) {
	w := newWorkspace(t)
	w.add("Secret plan", "1h")
	w.refresh()
	w.m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	text := plain(w)
	if !strings.Contains(text, "Make the window larger") || !strings.Contains(text, "80 × 24") || strings.Contains(text, "Secret plan") {
		t.Fatalf("got:\n%s", text)
	}
}

func TestErrorsAreShownPlainly(t *testing.T) {
	w := newWorkspace(t)
	w.m.Update(stateMsg{err: planner.ErrStillUnknown})
	if text := plain(w); !strings.Contains(text, "It is unknown whether the change was saved") {
		t.Fatalf("got:\n%s", text)
	}
}

// Whatever is open, every row is exactly as wide as the window and there are
// exactly as many rows, so a redraw never leaves stray characters behind.
func TestEveryScreenFitsTheWindow(t *testing.T) {
	long := func(t *testing.T) *workspace {
		w := newWorkspace(t)
		w.add(strings.Repeat("A very long title that keeps going ", 3), "1h")
		w.add("Short", "30m")
		w.refresh()
		return w
	}
	screens := map[string]func(t *testing.T) *workspace{
		"list":     long,
		"details":  func(t *testing.T) *workspace { w := long(t); w.press(key("enter")); return w },
		"footer":   func(t *testing.T) *workspace { w := long(t); w.press(key(".")); return w },
		"form":     func(t *testing.T) *workspace { w := long(t); w.press(key("e")); return w },
		"delete":   func(t *testing.T) *workspace { w := long(t); w.press(key("d")); return w },
		"calendar": func(t *testing.T) *workspace { w := long(t); w.press(key("c")); return w },
		"review":   func(t *testing.T) *workspace { w := long(t); w.press(key("r")); return w },
		"help":     func(t *testing.T) *workspace { w := long(t); w.press(key("?")); return w },
		"popup":    func(t *testing.T) *workspace { w := long(t); w.m.popup(domain.ErrDayClosed); return w },
		"order":    func(t *testing.T) *workspace { w := long(t); w.press(key("o"), key("J")); return w },
		"carry":    func(t *testing.T) *workspace { w := leftoverDays(t); w.run(w.press(key("C"))); return w },
		"mark done": func(t *testing.T) *workspace {
			w, _ := forgotten(t)
			w.press(key("f"))
			return w
		},
	}
	for name, open := range screens {
		w := open(t)
		for _, size := range [][2]int{{80, 24}, {120, 40}, {79, 24}, {40, 10}} {
			w.m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			rows := screen(w)
			if len(rows) != size[1] {
				t.Fatalf("%s %v: %d rows", name, size, len(rows))
			}
			for i, row := range rows {
				if got := ansi.StringWidth(row); got != size[0] {
					t.Fatalf("%s %v: row %d is %d wide: %q", name, size, i, got, row)
				}
			}
		}
	}
}

// Color is only used without NO_COLOR: teal for what is active and one
// color per status.
func TestColorFollowsNoColor(t *testing.T) {
	w := newWorkspace(t)
	w.add("Study", "1h")
	w.refresh()
	if strings.Contains(w.m.View().Content, "\x1b[") {
		t.Fatal("NO_COLOR output contains color codes")
	}
	w.m.style.noColor = false
	if content := w.m.View().Content; !strings.Contains(content, "\x1b[36mflowd") || !strings.Contains(content, "\x1b[36m> ") {
		t.Fatal("the brand and the selected item should be teal")
	}
	cases := map[domain.Status]string{domain.InProgress: "\x1b[34m", domain.Done: "\x1b[32m", domain.Paused: "\x1b[33m"}
	for status, color := range cases {
		if got := (style{}).status(status, "x"); got != color+"x"+reset {
			t.Errorf("%s is %q, want %q", status, got, color+"x"+reset)
		}
	}
}

// A frame takes well under a millisecond. The budget is far above that, so
// it only fails if drawing starts doing something expensive, like reading.
func TestDrawingAFrameIsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("timing budgets only run without -short")
	}
	w := newWorkspace(t)
	for i := 0; i < 30; i++ {
		w.add("Item", "30m")
	}
	w.refresh()
	start := time.Now()
	for i := 0; i < 100; i++ {
		w.m.recount()
		w.m.View()
	}
	if per := time.Since(start) / 100; per > 20*time.Millisecond {
		t.Errorf("a frame took %v, budget 20ms", per)
	}
}

// When the keys do not fit on one row, more [.] shows the rest below it and
// less [.] hides them again. Any other key folds them away. No key is lost.
func TestTheFooterFoldsAndNeverDropsAKey(t *testing.T) {
	w := newWorkspace(t)
	a := w.add("Study", "1h")
	w.act(planner.Start, a)
	w.refresh()
	w.m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	folded := plain(w)
	if !strings.Contains(folded, "more [.]") || strings.Contains(folded, "quit [q]") {
		t.Fatalf("folded:\n%s", folded)
	}
	w.press(key("."))
	open := plain(w)
	for _, want := range w.m.hints() {
		if !strings.Contains(open, want) {
			t.Errorf("%q is missing when the footer is open:\n%s", want, open)
		}
	}
	if !strings.Contains(open, "less [.]") {
		t.Fatalf("open:\n%s", open)
	}
	w.press(key("j"))
	if strings.Contains(plain(w), "less [.]") {
		t.Fatal("any other key should fold the footer again")
	}
	w.m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	if text := plain(w); strings.Contains(text, "more [.]") || !strings.Contains(text, "quit [q]") {
		t.Fatalf("a wide window needs no more [.]:\n%s", text)
	}
}

// The footer names what space does for the selected item, and offers carry
// whenever the day can be planned.
func TestTheFooterOffersWhatTheItemAllows(t *testing.T) {
	w := newWorkspace(t)
	id := w.add("Study", "1h")
	hints := func() string { w.refresh(); return strings.Join(w.m.hints(), " ") }
	for _, step := range []struct {
		op   planner.Operation
		want string
	}{{"", "start [space]"}, {planner.Start, "pause [space] finish [f]"}, {planner.Pause, "resume [space] finish [f]"}, {planner.Finish, "reopen [space]"}} {
		if step.op != "" {
			w.now = w.now.Add(time.Minute)
			w.act(step.op, id)
		}
		if got := hints(); !strings.Contains(got, step.want) || !strings.Contains(got, "carry [C]") {
			t.Fatalf("after %q: %s", step.op, got)
		}
	}
	w.m.date = "2026-11-16"
	if got := hints(); strings.Contains(got, "carry [C]") {
		t.Fatalf("a past day cannot carry: %s", got)
	}
}
