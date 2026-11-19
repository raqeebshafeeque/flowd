package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/planner"
)

func view(id, title string, status domain.Status, minutes int64) planner.ItemView {
	return planner.ItemView{Item: domain.Item{ID: domain.ID(id), Title: title, Status: status, DurationMinutes: minutes}}
}

func TestTheTableLooksLikeTheWorkspace(t *testing.T) {
	var out strings.Builder
	items := []planner.ItemView{view("it_A", "Write release notes", domain.Todo, 30), view("it_B", "Inbox", domain.Done, 20)}
	if err := printTable(&out, look{}, items); err != nil {
		t.Fatal(err)
	}
	want := `┌──────┬─────────────────────┬──────────┬────────┐
│ ID   │ TITLE               │ DURATION │ STATUS │
├──────┼─────────────────────┼──────────┼────────┤
│ it_A │ Write release notes │      30m │   TODO │
│ it_B │ Inbox               │      20m │   DONE │
└──────┴─────────────────────┴──────────┴────────┘
`
	if out.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", out.String(), want)
	}
}

func TestALongTitleIsShortenedToFitTheTerminal(t *testing.T) {
	var out strings.Builder
	title := strings.Repeat("A very long title ", 6)
	printTable(&out, look{width: 60}, []planner.ItemView{view("it_A", title, domain.Todo, 30)})
	for _, row := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		if w := ansi.StringWidth(row); w != 60 {
			t.Fatalf("row is %d wide, want 60: %q", w, row)
		}
	}
	if !strings.Contains(out.String(), "…") {
		t.Fatal("a shortened title should end in an ellipsis")
	}
}

func TestStatusesAreColoredOnlyWhenAllowed(t *testing.T) {
	items := []planner.ItemView{view("it_A", "Study", domain.InProgress, 30)}
	var plain, colored strings.Builder
	printTable(&plain, look{}, items)
	printTable(&colored, look{color: true}, items)
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatal("plain output has color codes")
	}
	if !strings.Contains(colored.String(), "\x1b[34mIN PROGRESS"+reset) {
		t.Fatalf("in progress should be blue: %q", colored.String())
	}
}

func TestADeletedItemSaysSo(t *testing.T) {
	item := view("it_A", "Study", domain.Todo, 30)
	item.Archived = true
	var out strings.Builder
	printTable(&out, look{color: true}, []planner.ItemView{item})
	if !strings.Contains(out.String(), "DELETED │") {
		t.Fatalf("got %q", out.String())
	}
}
