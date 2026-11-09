package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

func (f *commandFixture) saved(args ...string) domain.Block {
	f.t.Helper()
	var block domain.Block
	if err := json.Unmarshal([]byte(f.ok(append([]string{"--json"}, args...)...)), &block); err != nil {
		f.t.Fatal(err)
	}
	return block
}
func (f *commandFixture) view(args ...string) dayView {
	f.t.Helper()
	var view dayView
	if err := json.Unmarshal([]byte(f.ok(append([]string{"--json"}, args...)...)), &view); err != nil {
		f.t.Fatal(err)
	}
	return view
}

func TestAddCommand(t *testing.T) {
	f := newCommandFixture(t)
	title := "Go's notes; DROP TABLE blocks; --"
	block := f.saved("add", "09:00", "10:00", title)
	if !strings.HasPrefix(string(block.ID), "bl_") || block.Title != title || block.Status != domain.Todo || !block.CreatedAt.Equal(f.now) {
		t.Error("add should create a stamped todo with a fresh id and literal title")
	}
	view := f.view()
	if len(view.Blocks) != 1 || view.Blocks[0].ID != block.ID {
		t.Fatal("new plan must survive the next command")
	}
	f.fail("ERR_OVERLAP", 1, "add", "09:30", "10:30", "Overlapping")
	if len(f.view().Blocks) != 1 {
		t.Error("refused addition must not change the plan")
	}
	unicodeTitle := strings.Repeat("界", 120)
	if got := f.saved("add", "10:00", "11:00", "  "+unicodeTitle+"  "); got.Title != unicodeTitle {
		t.Error("title limits must apply after trimming and count characters")
	}
}

func TestInvalidFirstAddition(t *testing.T) {
	for _, test := range []struct{ name, start, end, title, code string }{
		{"backwards", "10:00", "09:00", "Study", "ERR_TIME"},
		{"bad clock", "25:00", "26:00", "Study", "ERR_USAGE"},
		{"empty title", "09:00", "10:00", " ", "ERR_TITLE"},
		{"multiline title", "09:00", "10:00", "Study\nCode", "ERR_USAGE"},
		{"unicode line separator", "09:00", "10:00", "Study\u2028Code", "ERR_USAGE"},
		{"invalid unicode", "09:00", "10:00", string([]byte{0xff}), "ERR_USAGE"},
		{"long title", "09:00", "10:00", strings.Repeat("a", 121), "ERR_TITLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newCommandFixture(t)
			f.fail(test.code, 2, "add", test.start, test.end, test.title)
			db, err := store.Open(context.Background(), f.path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.DayByDate(context.Background(), "2026-10-14"); !errors.Is(err, store.ErrDayNotFound) {
				t.Error("bad first addition must not leave a partial day")
			}
		})
	}
}

func TestEditCommand(t *testing.T) {
	f := newCommandFixture(t)
	first := f.saved("add", "09:00", "10:00", "Study")
	f.saved("add", "10:00", "11:00", "Code")
	f.fail("ERR_OVERLAP", 1, "edit", "--title", "Wrong", "--end", "10:15", "1")
	saved := f.view().Blocks[0]
	if saved.Title != first.Title || !saved.PlannedEnd.Equal(first.PlannedEnd) || !saved.CreatedAt.Equal(first.CreatedAt) || saved.UpdatedAt != nil {
		t.Error("failed edit must retain title, interval and metadata")
	}
	db, err := store.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	events, err := db.EventsByBlock(context.Background(), first.ID)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Reason != domain.ReasonUser {
		t.Error("creation needs initial history and refused edit adds none")
	}
	changed := f.saved("edit", "--title", "Deep Study", "--start", "08:45", string(first.ID))
	if changed.Title != "Deep Study" || changed.PlannedStart.Format("15:04") != "08:45" || changed.UpdatedAt == nil || !changed.UpdatedAt.Equal(f.now) {
		t.Error("edit must save all requested fields with the command instant")
	}
}

func TestDeleteCommand(t *testing.T) {
	f := newCommandFixture(t)
	first := f.saved("add", "09:00", "10:00", "Study")
	second := f.saved("add", "10:00", "11:00", "Code")
	archived := f.saved("delete", "1")
	if !archived.Archived {
		t.Error("delete should archive rather than remove")
	}
	if rows := f.view().Blocks; len(rows) != 1 || rows[0].ID != second.ID {
		t.Fatal("visible row order must close the gap")
	}
	if got := f.saved("delete", string(first.ID)); !got.Archived {
		t.Error("repeated full-id deletion must succeed")
	}
	db, err := store.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	events, err := db.EventsByBlock(context.Background(), first.ID)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Errorf("got %d events, want creation plus one archive", len(events))
	}
	f.saved("add", "09:00", "10:00", "Replacement")
}

func TestResolveBlock(t *testing.T) {
	f := newCommandFixture(t)
	first := f.blockFixture("bl_first", "09:00", "10:00")
	second := f.blockFixture("bl_second", "10:00", "11:00")
	hidden := f.blockFixture("bl_hidden", "11:00", "12:00")
	hidden.Archived = true
	day := f.seed("2026-10-14", "UTC", first, second, hidden)
	other := f.blockFixture("bl_other", "09:00", "10:00")
	other.PlannedStart = other.PlannedStart.AddDate(0, 0, 1)
	other.PlannedEnd = other.PlannedEnd.AddDate(0, 0, 1)
	f.seed("2026-10-15", "UTC", other)
	db, err := store.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	blocks, err := db.BlocksByDay(context.Background(), day.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, selector, want string
		archived, invalid    bool
	}{
		{"row number", "1", "bl_first", false, false},
		{"full id", "bl_second", "bl_second", false, false},
		{"unique prefix", "bl_f", "bl_first", false, false},
		{"ambiguous prefix", "bl_", "", false, true},
		{"empty selector", "", "", false, true},
		{"missing row", "99", "", false, true},
		{"zero row", "0", "", false, true},
		{"cross day", "bl_other", "", true, true},
		{"hidden target", "bl_hidden", "", false, true},
		{"hidden history", "bl_hidden", "bl_hidden", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveBlock(context.Background(), db, day, blocks, test.selector, test.archived)
			if test.invalid {
				if err == nil {
					t.Error("selector must be refused")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got.ID) != test.want {
				t.Errorf("got id %q, want %q", got.ID, test.want)
			}
		})
	}
}
