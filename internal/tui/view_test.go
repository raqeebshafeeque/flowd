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

// Every row must be exactly as wide as the window, at every supported size.
func TestEveryRowFillsTheWindowExactly(t *testing.T) {
	w := newWorkspace(t)
	w.add(strings.Repeat("A very long title that keeps going ", 3), "1h")
	w.add("Short", "30m")
	w.refresh()
	w.press(key("j"), key("enter"))
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 40}, {79, 24}, {80, 23}, {40, 10}} {
		w.m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		rows := screen(w)
		if len(rows) != size[1] {
			t.Fatalf("%v: %d rows", size, len(rows))
		}
		for i, row := range rows {
			if got := ansi.StringWidth(row); got != size[0] {
				t.Fatalf("%v: row %d is %d wide: %q", size, i, got, row)
			}
		}
	}
}

func TestTheListShowsWorkDurationAndStatus(t *testing.T) {
	w := newWorkspace(t)
	a := w.add("Study", "1h")
	w.add("Code", "30m")
	w.act(planner.Start, a)
	w.now = w.now.Add(10 * time.Minute)
	w.refresh()
	text := plain(w)
	for _, want := range []string{"flowd", "Tue 17 Nov 2026 │ 09:10:00", "> Study", "10m / 1h", "IN PROGRESS", "Code", "30m", "TODO", "pause [space]  finish [f]  skip [s]", "help [?]", "quit [q]"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Index(text, "Study") > strings.Index(text, "Code") {
		t.Error("the started item must come first")
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
	for _, want := range []string{"Details", "Title        Study the scheduler", "Duration     1h30m", "Work         5m6s", "Elapsed      5m6s", "Started At   17 Nov 2026 09:00:00", "Finished At  -", "close [esc]"} {
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

func TestColorFollowsNoColor(t *testing.T) {
	w := newWorkspace(t)
	w.add("Study", "1h")
	w.refresh()
	if strings.Contains(w.m.View().Content, "\x1b[") {
		t.Fatal("NO_COLOR output contains color codes")
	}
	w.m.style.noColor = false
	if !strings.Contains(w.m.View().Content, "\x1b[36mflowd") {
		t.Fatal("the brand should be teal in color mode")
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

func TestTimingAndStatusSitAtTheRightEdge(t *testing.T) {
	w := newWorkspace(t)
	a := w.add("Study", "1h")
	w.add("Code", "30m")
	w.act(planner.Start, a)
	w.refresh()
	for _, row := range screen(w)[3:5] {
		plainRow := ansi.Strip(row)
		if !strings.HasSuffix(plainRow, "IN PROGRESS │") && !strings.HasSuffix(plainRow, "       TODO │") {
			t.Errorf("the status is not at the right edge: %q", plainRow)
		}
	}
	w.m.style.noColor = false
	if !strings.Contains(w.m.View().Content, "\x1b[36m> ") {
		t.Error("the selected item should be teal")
	}
}

// The header has four cells, flowd, a note, the date and the clock, and the
// dividers between them meet the borders above and below.
func TestTheHeaderIsSplitIntoJoinedCells(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	for _, size := range [][2]int{{80, 24}, {100, 30}, {160, 40}} {
		w.m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		rows := strings.Split(plain(w), "\n")
		top, header, below := []rune(rows[0]), []rune(rows[1]), []rune(rows[2])
		var dividers []int
		for i, r := range header {
			if r == '│' && i > 0 && i < len(header)-1 {
				dividers = append(dividers, i)
			}
		}
		if len(dividers) != 3 {
			t.Fatalf("%v: want 3 dividers in %q", size, rows[1])
		}
		for _, i := range dividers {
			if top[i] != '┬' || below[i] != '┴' {
				t.Fatalf("%v: divider at %d is not joined: %q / %q", size, i, string(top[i]), string(below[i]))
			}
		}
		if !strings.HasPrefix(rows[1], "│ flowd │") || !strings.HasSuffix(rows[1], "│ Tue 17 Nov 2026 │ 09:00:00 │") {
			t.Fatalf("%v: unexpected header %q", size, rows[1])
		}
		if middle := string(header[dividers[0]+1 : dividers[1]]); strings.TrimSpace(middle) != "" {
			t.Fatalf("today should leave the middle empty, got %q", middle)
		}
	}
	w.press(key("c"), key("l"))
	w.run(w.press(key("enter")))
	rows := strings.Split(plain(w), "\n")
	if !strings.HasPrefix(rows[1], "│ flowd │ Viewing: 2026-11-18 ") {
		t.Fatalf("another day should be named in the middle: %q", rows[1])
	}
}

func TestStatusColors(t *testing.T) {
	s := style{}
	cases := map[domain.Status]string{domain.InProgress: "\x1b[34m", domain.Done: "\x1b[32m", domain.Paused: "\x1b[33m"}
	for status, color := range cases {
		if got := s.status(status, "x"); got != color+"x"+reset {
			t.Errorf("%s is %q, want %q", status, got, color+"x"+reset)
		}
	}
	if (style{noColor: true}).status(domain.Done, "DONE") != "DONE" {
		t.Error("NO_COLOR must stay plain")
	}
}
