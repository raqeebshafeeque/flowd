package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/utils"
)

func mkTime(h, m int) time.Time {
	return time.Date(2026, time.October, 14, h, m, 0, 0, time.UTC)
}

func TestIsOverlapping(t *testing.T) {
	day := []Block{
		{ID: "study", DayID: "day-1", Title: "study", Status: Todo, PlannedStart: mkTime(9, 0), PlannedEnd: mkTime(10, 0)},
		{ID: "code", DayID: "day-1", Title: "code", Status: Paused, PlannedStart: mkTime(10, 30), PlannedEnd: mkTime(11, 30)},
		{ID: "gym", DayID: "day-1", Title: "gym", Status: Done, PlannedStart: mkTime(14, 0), PlannedEnd: mkTime(15, 0)},
		{ID: "read", DayID: "day-1", Title: "read", Status: Skipped, PlannedStart: mkTime(16, 0), PlannedEnd: mkTime(17, 0)},
		{ID: "old", DayID: "day-1", Title: "old", Status: Todo, PlannedStart: mkTime(12, 0), PlannedEnd: mkTime(13, 0), Archived: true},
	}

	tests := []struct {
		name   string
		start  time.Time
		end    time.Time
		others []Block
		want   bool
	}{
		{name: "fully inside a block", start: mkTime(9, 15), end: mkTime(9, 45), others: day, want: true},
		{name: "start inside a block", start: mkTime(9, 30), end: mkTime(10, 15), others: day, want: true},
		{name: "end inside a block", start: mkTime(8, 30), end: mkTime(9, 30), others: day, want: true},
		{name: "spans a whole block", start: mkTime(8, 0), end: mkTime(10, 0), others: day, want: true},
		{name: "identical to a block", start: mkTime(9, 0), end: mkTime(10, 0), others: day, want: true},
		{name: "inside a skipped block", start: mkTime(16, 15), end: mkTime(16, 45), others: day, want: true},
		{name: "touching a block end", start: mkTime(10, 0), end: mkTime(10, 30), others: day, want: false},
		{name: "touching a block start", start: mkTime(8, 0), end: mkTime(9, 0), others: day, want: false},
		{name: "sitting in a gap", start: mkTime(13, 0), end: mkTime(14, 0), others: day, want: false},
		{name: "after every block", start: mkTime(17, 0), end: mkTime(18, 0), others: day, want: false},
		{name: "before every block", start: mkTime(7, 0), end: mkTime(8, 0), others: day, want: false},
		{name: "empty day never overlaps", start: mkTime(9, 0), end: mkTime(10, 0), others: nil, want: false},
		{name: "archived block holds no time", start: mkTime(12, 15), end: mkTime(12, 45), others: day, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOverlapping(tt.start, tt.end, tt.others); got != tt.want {
				t.Errorf("got overlap %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateStatus(t *testing.T) {
	sameDay := Day{ID: "day", Date: "2026-10-14", Timezone: "Asia/Kolkata"}
	otherDay := Day{ID: "day", Date: "2026-10-13", Timezone: "Asia/Kolkata"}
	tests := []struct {
		name    string
		from    Status
		to      Status
		day     Day
		want    Status
		wantErr error
	}{
		{name: "todo starts running", from: Todo, to: InProgress, want: InProgress},
		{name: "todo gives up", from: Todo, to: Skipped, want: Skipped},
		{name: "running pauses", from: InProgress, to: Paused, want: Paused},
		{name: "running finishes", from: InProgress, to: Done, want: Done},
		{name: "running gives up", from: InProgress, to: Skipped, want: Skipped},
		{name: "paused resumes", from: Paused, to: InProgress, want: InProgress},
		{name: "paused finishes", from: Paused, to: Done, want: Done},
		{name: "paused gives up", from: Paused, to: Skipped, want: Skipped},

		{name: "todo cannot finish unstarted", from: Todo, to: Done, wantErr: ErrIllegalTransition},
		{name: "todo cannot pause unstarted", from: Todo, to: Paused, wantErr: ErrIllegalTransition},
		{name: "todo cannot redo itself", from: Todo, to: Todo, wantErr: ErrIllegalTransition},
		{name: "running cannot restart", from: InProgress, to: InProgress, wantErr: ErrIllegalTransition},
		{name: "running cannot return to todo", from: InProgress, to: Todo, wantErr: ErrIllegalTransition},
		{name: "paused cannot return to todo", from: Paused, to: Todo, wantErr: ErrIllegalTransition},
		{name: "paused cannot pause again", from: Paused, to: Paused, wantErr: ErrIllegalTransition},
		{name: "done cannot finish again", from: Done, to: Done, wantErr: ErrIllegalTransition},
		{name: "done cannot pause", from: Done, to: Paused, wantErr: ErrIllegalTransition},
		{name: "done cannot skip", from: Done, to: Skipped, wantErr: ErrIllegalTransition},
		{name: "done cannot return to todo", from: Done, to: Todo, wantErr: ErrIllegalTransition},
		{name: "done reopens same day", from: Done, to: InProgress, day: sameDay, want: InProgress},
		{name: "done refuses other day", from: Done, to: InProgress, day: otherDay, wantErr: ErrIllegalTransition},
		{name: "skipped is terminal", from: Skipped, to: InProgress, wantErr: ErrIllegalTransition},
		{name: "skipped cannot finish", from: Skipped, to: Done, wantErr: ErrIllegalTransition},
		{name: "skipped cannot skip again", from: Skipped, to: Skipped, wantErr: ErrIllegalTransition},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := Block{ID: "block", DayID: "day", Title: "block", Status: tt.from}
			now := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
			day := tt.day
			if day.Date == "" {
				day = sameDay
			}
			err := b.UpdateStatus(tt.to, now, day)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if b.Status != tt.want {
				t.Errorf("got status %q, want %q", b.Status, tt.want)
			}
		})
	}
}

func TestNewBlockInputValidation(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		input   NewBlockInput
		wantErr error
	}{
		{
			name: "empty title",
			input: NewBlockInput{
				Title:          "",
				DayID:          "day",
				PlannedStartAt: now,
				PlannedEndAt:   now.Add(time.Hour),
				CopiedFromID:   nil,
			}, wantErr: ErrInvalidBlockTitle,
		},
		{
			name: "title with white spaces only",
			input: NewBlockInput{
				Title:          "    ",
				DayID:          "day",
				PlannedStartAt: now,
				PlannedEndAt:   now.Add(time.Hour),
				CopiedFromID:   nil,
			}, wantErr: ErrInvalidBlockTitle,
		},
		{
			name: "title with size greater than 120",
			input: NewBlockInput{
				Title:          strings.Repeat("abc", 50),
				DayID:          "day",
				PlannedStartAt: now,
				PlannedEndAt:   now.Add(time.Hour),
				CopiedFromID:   nil,
			}, wantErr: ErrInvalidBlockTitle,
		},
		{
			name: "start at and end at are same",
			input: NewBlockInput{
				Title:          "study",
				DayID:          "day",
				PlannedStartAt: now,
				PlannedEndAt:   now,
				CopiedFromID:   nil,
			}, wantErr: ErrInvalidStartAndEnd,
		},
		{
			name: "start at after end at",
			input: NewBlockInput{
				Title:          "code",
				DayID:          "day",
				PlannedStartAt: now.Add(time.Hour),
				PlannedEndAt:   now,
				CopiedFromID:   nil,
			}, wantErr: ErrInvalidStartAndEnd,
		},
		{
			name: "all valid",
			input: NewBlockInput{
				Title:          "read",
				DayID:          "day",
				PlannedStartAt: now,
				PlannedEndAt:   now.Add(time.Hour),
				CopiedFromID:   nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("got error %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && err != nil {
				t.Errorf("unexpected error %v", err)
			}
		})
	}
}

func TestNewBlock(t *testing.T) {
	now := time.Date(2026, time.October, 14, 9, 0, 0, 0, time.UTC)
	t.Run("bad input returns nil plus the error", func(t *testing.T) {
		got, err := NewBlock(NewBlockInput{Title: "", DayID: "day", PlannedStartAt: now, PlannedEndAt: now.Add(time.Hour)})
		if !errors.Is(err, ErrInvalidBlockTitle) {
			t.Fatalf("got error %v, want %v", err, ErrInvalidBlockTitle)
		}
		if got != nil {
			t.Errorf("got non-nil block, want nil")
		}
	})

	t.Run("good input builds a todo block", func(t *testing.T) {
		copied := utils.ID("bl_old")
		got, err := NewBlock(NewBlockInput{Title: "  study  ", DayID: "day", PlannedStartAt: now, PlannedEndAt: now.Add(time.Hour), CopiedFromID: &copied})
		if err != nil {
			t.Fatalf("unexpected error %v", err)
		}
		if !strings.HasPrefix(string(got.ID), "bl_") {
			t.Errorf("got ID %q, want prefix %q", got.ID, "bl_")
		}
		if got.Status != Todo {
			t.Errorf("got status %q, want %q", got.Status, Todo)
		}
		if got.Title != "study" {
			t.Errorf("got title %q, want trimmed %q", got.Title, "study")
		}
		if !got.PlannedStart.Equal(now) || !got.PlannedEnd.Equal(now.Add(time.Hour)) {
			t.Errorf("got times %v to %v, want %v to %v", got.PlannedStart, got.PlannedEnd, now, now.Add(time.Hour))
		}
		if got.DayID != "day" || got.CopiedFromID == nil || *got.CopiedFromID != copied {
			t.Errorf("got day %q copy from %v, want day and bl_old", got.DayID, got.CopiedFromID)
		}
		if got.ActualStart != nil || got.ActualEnd != nil || got.Archived {
			t.Errorf("want nil actuals and unarchived")
		}
		if got.CreatedAt.IsZero() {
			t.Errorf("got zero CreatedAt, want stamp")
		}
	})
}
