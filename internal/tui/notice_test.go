package tui

import (
	"strings"
	"testing"
	"time"

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
