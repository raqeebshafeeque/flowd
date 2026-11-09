package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReopenLocalDate(t *testing.T) {
	for _, test := range []struct {
		name, zone, now string
		want            error
	}{
		{"utc yesterday is local today", "Asia/Kolkata", "2026-10-13T20:00:00Z", nil},
		{"utc tomorrow is local today", "America/Los_Angeles", "2026-10-15T03:00:00Z", nil},
		{"local day already ended", "Asia/Kolkata", "2026-10-14T20:00:00Z", ErrIllegalTransition},
		{"local day has not started", "America/Los_Angeles", "2026-10-14T03:00:00Z", ErrIllegalTransition},
		{"invalid saved zone", "not/a-zone", "2026-10-14T12:00:00Z", ErrInvalidDayZone},
		{"empty saved zone", "", "2026-10-14T12:00:00Z", ErrInvalidDayZone},
	} {
		t.Run(test.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, test.now)
			if err != nil {
				t.Fatal(err)
			}
			ended := timeFromStr("10:00")
			block := Block{Status: Done, ActualEnd: &ended}
			err = block.UpdateStatus(InProgress, now, Day{Date: "2026-10-14", Timezone: test.zone})
			if !errors.Is(err, test.want) {
				t.Fatalf("got error %v, want %v", err, test.want)
			}
			if err == nil {
				if block.ActualEnd != nil || block.Status != InProgress {
					t.Error("reopen should clear finish time")
				}
			} else if block.Status != Done || block.ActualEnd != &ended {
				t.Error("refused reopen should not mutate the block")
			}
		})
	}
}

func TestCommandRuleBoundaries(t *testing.T) {
	t.Run("trimmed Unicode title limit", func(t *testing.T) {
		input := NewBlockInput{Title: "  " + strings.Repeat("界", 120) + "  ", DayID: "day", PlannedStartAt: timeFromStr("09:00"), PlannedEndAt: timeFromStr("10:00")}
		block, err := NewBlock(input)
		if err != nil {
			t.Fatal(err)
		}
		if block.Title != strings.TrimSpace(input.Title) {
			t.Error("title should be trimmed")
		}
		input.Title = strings.Repeat("界", 121)
		if _, err := NewBlock(input); !errors.Is(err, ErrInvalidBlockTitle) {
			t.Errorf("want title limit, got %v", err)
		}
	})
	t.Run("archived blocks cannot transition or extend", func(t *testing.T) {
		block := Block{Archived: true, Status: InProgress, PlannedStart: timeFromStr("09:00"), PlannedEnd: timeFromStr("10:00")}
		if err := block.UpdateStatus(Paused, timeFromStr("09:30"), Day{}); !errors.Is(err, ErrIllegalTransition) {
			t.Errorf("want archived refusal, got %v", err)
		}
		if err := block.Extend(nil); !errors.Is(err, ErrIllegalTransition) {
			t.Errorf("want archived refusal, got %v", err)
		}
		if block.Status != InProgress || !block.PlannedEnd.Equal(timeFromStr("10:00")) {
			t.Error("archived block must stay unchanged")
		}
	})
}
