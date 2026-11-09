package cli

import (
	"context"
	"encoding/json"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
	"testing"
	"time"
)

func TestReviewCommand(t *testing.T) {
	f := newCommandFixture(t)
	for i, clock := range []string{"08:00", "09:00", "10:00", "11:00", "12:00"} {
		start := commandTime(t, "2026-10-14T"+clock+":00Z")
		end := start.Add(time.Hour)
		f.saved("add", start.Format("15:04"), end.Format("15:04"), "Work")
		if i < 3 {
			f.saved("start", string(rune('1'+i)))
			f.now = f.now.Add(time.Hour)
			f.saved("finish", string(rune('1'+i)))
		}
		if i == 3 {
			f.saved("skip", "4")
		}
	}
	var review struct {
		Done, Skipped, Unfinished int
		Progress                  *int
		Missed, Overruns          []string
	}
	read := func() {
		t.Helper()
		if err := json.Unmarshal([]byte(f.ok("--date", "2026-10-14", "--json", "review")), &review); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if review.Progress == nil || *review.Progress != 75 || review.Done != 3 || review.Skipped != 1 || review.Unfinished != 1 || len(review.Missed) != 0 || len(review.Overruns) == 0 {
		t.Error("review needs the documented 75-percent score, counts and completed overruns without premature missed todos")
	}
	f.now = commandTime(t, "2026-10-15T00:00:00Z")
	read()
	if len(review.Missed) != 1 {
		t.Error("unfinished todo becomes missed after local day end")
	}
}

func TestUncountableReview(t *testing.T) {
	f := newCommandFixture(t)
	for _, skipped := range []bool{false, true} {
		if skipped {
			f.saved("add", "09:00", "10:00", "Study")
			f.saved("skip", "1")
		}
		var review struct{ Progress *int }
		if err := json.Unmarshal([]byte(f.ok("--json", "review")), &review); err != nil {
			t.Fatal(err)
		}
		if review.Progress != nil {
			t.Error("empty and all-skipped scores must be null")
		}
	}
}

func TestCopyCalendarEdges(t *testing.T) {
	for _, test := range []struct{ name, date, start, end, wantStart, wantEnd string }{
		{"spring day", "2026-03-07", "09:00", "10:00", "2026-03-08T13:00:00Z", "2026-03-08T14:00:00Z"},
		{"overnight spring", "2026-03-07", "23:00", "2026-03-08T01:00:00-05:00", "2026-03-09T03:00:00Z", "2026-03-09T05:00:00Z"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newCommandFixture(t)
			flags := []string{"--date", test.date, "--timezone", "America/New_York"}
			f.saved(append(flags, "add", test.start, test.end, "Study")...)
			copied := f.saved(append(flags, "copy", "1")...)
			if !copied.PlannedStart.Equal(commandTime(t, test.wantStart)) || !copied.PlannedEnd.Equal(commandTime(t, test.wantEnd)) {
				t.Error("calendar copy must preserve local clocks and overnight end position")
			}
		})
	}
	t.Run("ambiguous target", func(t *testing.T) {
		f := newCommandFixture(t)
		flags := []string{"--date", "2026-10-31", "--timezone", "America/New_York"}
		f.saved(append(flags, "add", "01:30", "02:30", "Study")...)
		f.fail("ERR_USAGE", 2, append(flags, "copy", "1")...)
		f.saved(append(flags, "copy", "--start", "2026-11-01T01:30:00-05:00", "1")...)
	})
	for _, command := range []string{"finish", "skip", "delete"} {
		t.Run("refuse "+command, func(t *testing.T) {
			f := newCommandFixture(t)
			flags := []string{"--date", "2026-03-07", "--timezone", "America/New_York"}
			f.saved(append(flags, "add", "02:30", "03:30", "Study")...)
			source := f.saved(append(flags, "start", "1")...)
			f.saved(append(flags, command, "1")...)
			f.fail("ERR_CARRYOVER", 1, append(flags, "copy", string(source.ID))...)
		})
	}
}

func TestCopyCommand(t *testing.T) {
	f := newCommandFixture(t)
	source := f.saved("add", "09:00", "10:00", "Study")
	f.saved("start", "1")
	copied := f.saved("copy", "1")
	if copied.ID == source.ID || copied.CopiedFromID == nil || *copied.CopiedFromID != source.ID || copied.Status != domain.Todo || copied.ActualStart != nil || copied.ActualEnd != nil || copied.PlannedStart.Format("2006-01-02 15:04") != "2026-10-15 09:00" {
		t.Error("copy should be a fresh next-day todo with provenance and no actuals")
	}
	second := f.saved("copy", "--start", "11:00", "--end", "12:00", "1")
	if second.ID == copied.ID {
		t.Error("distinct free copies need distinct ids")
	}
	f.fail("ERR_OVERLAP", 1, "copy", "1")
	db, err := store.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	events, err := db.EventsByBlock(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Error("copy must not append source history")
	}
	target, err := db.DayByDate(context.Background(), "2026-10-15")
	if err != nil {
		t.Fatal(err)
	}
	if target.ID != copied.DayID {
		t.Error("copy must persist its new day with the block")
	}
}
