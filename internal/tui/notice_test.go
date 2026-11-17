package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func TestErrorsShowForSevenSeconds(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.m.popup(domain.ErrDayClosed)
	if !strings.Contains(plain(w), "That day is not open") {
		t.Fatal("the popup is not shown")
	}
	w.now = w.now.Add(6 * time.Second)
	if w.m.visibleNotice() == nil {
		t.Fatal("the popup vanished too early")
	}
	w.now = w.now.Add(time.Second)
	if w.m.visibleNotice() != nil || strings.Contains(plain(w), "That day is not open") {
		t.Fatal("the popup outlived seven seconds")
	}
}

func TestThePopupFitsTheWindow(t *testing.T) {
	w := newWorkspace(t)
	w.refresh()
	w.m.popup(domain.ErrDayClosed)
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		w.m.width, w.m.height = size[0], size[1]
		for i, row := range screen(w) {
			if ansi.StringWidth(row) != size[0] {
				t.Fatalf("%v: row %d is %d wide", size, i, ansi.StringWidth(row))
			}
		}
	}
}
