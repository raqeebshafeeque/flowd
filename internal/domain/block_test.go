package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/utils"
)

func timeFromStr(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", "2026-10-14 "+s)
	if err != nil {
		panic("bad test clock: " + s)
	}
	return t
}

func TestIsOverlapping(t *testing.T) {
	day := []Block{
		{ID: "study", DayID: "day", Title: "study", Status: Todo, PlannedStart: timeFromStr("09:00"), PlannedEnd: timeFromStr("10:00")},
		{ID: "code", DayID: "day", Title: "code", Status: Paused, PlannedStart: timeFromStr("10:30"), PlannedEnd: timeFromStr("11:30")},
		{ID: "gym", DayID: "day", Title: "gym", Status: Done, PlannedStart: timeFromStr("14:00"), PlannedEnd: timeFromStr("15:00")},
		{ID: "read", DayID: "day", Title: "read", Status: Skipped, PlannedStart: timeFromStr("16:00"), PlannedEnd: timeFromStr("17:00")},
		{ID: "old", DayID: "day", Title: "old", Status: Todo, PlannedStart: timeFromStr("12:00"), PlannedEnd: timeFromStr("13:00"), Archived: true},
	}

	tests := []struct {
		name   string
		start  time.Time
		end    time.Time
		others []Block
		want   bool
	}{
		{name: "fully inside a block", start: timeFromStr("09:15"), end: timeFromStr("09:45"), others: day, want: true},
		{name: "start inside a block", start: timeFromStr("09:30"), end: timeFromStr("10:15"), others: day, want: true},
		{name: "end inside a block", start: timeFromStr("08:30"), end: timeFromStr("09:30"), others: day, want: true},
		{name: "spans a whole block", start: timeFromStr("08:00"), end: timeFromStr("10:00"), others: day, want: true},
		{name: "identical to a block", start: timeFromStr("09:00"), end: timeFromStr("10:00"), others: day, want: true},
		{name: "inside a skipped block", start: timeFromStr("16:15"), end: timeFromStr("16:45"), others: day, want: true},
		{name: "touching a block end", start: timeFromStr("10:00"), end: timeFromStr("10:30"), others: day, want: false},
		{name: "touching a block start", start: timeFromStr("08:00"), end: timeFromStr("09:00"), others: day, want: false},
		{name: "sitting in a gap", start: timeFromStr("13:00"), end: timeFromStr("14:00"), others: day, want: false},
		{name: "after every block", start: timeFromStr("17:00"), end: timeFromStr("18:00"), others: day, want: false},
		{name: "before every block", start: timeFromStr("07:00"), end: timeFromStr("08:00"), others: day, want: false},
		{name: "empty day never overlaps", start: timeFromStr("09:00"), end: timeFromStr("10:00"), others: nil, want: false},
		{name: "archived block holds no time", start: timeFromStr("12:15"), end: timeFromStr("12:45"), others: day, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOverlapping(tt.start, tt.end, tt.others); got != tt.want {
				t.Errorf("got overlap %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtend(t *testing.T) {
	study := Block{ID: "study", DayID: "day", Title: "study", Status: InProgress, PlannedStart: timeFromStr("09:00"), PlannedEnd: timeFromStr("10:00")}
	code := Block{ID: "code", DayID: "day", Title: "code", Status: Todo, PlannedStart: timeFromStr("10:30"), PlannedEnd: timeFromStr("11:30")}
	day := []Block{study, code}
	tests := []struct {
		name    string
		block   Block
		day     []Block
		wantEnd time.Time
		wantErr error
	}{
		{name: "stretch lands in free space", block: study, day: day, wantEnd: timeFromStr("10:15")},
		{name: "stretch skips itself", block: Block{ID: "solo", DayID: "other", Title: "solo", Status: Paused, PlannedStart: timeFromStr("09:00"), PlannedEnd: timeFromStr("10:00")}, day: []Block{
			{ID: "solo", DayID: "other", Title: "solo", Status: Paused, PlannedStart: timeFromStr("09:00"), PlannedEnd: timeFromStr("10:00")},
		}, wantEnd: timeFromStr("10:15")},
		{name: "stretch into clash refuses", block: study, day: []Block{
			study,
			{ID: "code", DayID: "day", Title: "code", Status: Todo, PlannedStart: timeFromStr("10:00"), PlannedEnd: timeFromStr("11:00")},
		}, wantErr: ErrOverlap},
		{name: "todo cannot stretch", block: code, day: day, wantErr: ErrIllegalTransition},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.block
			err := b.Extend(tt.day)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if !b.PlannedEnd.Equal(tt.wantEnd) {
				t.Errorf("got end %v, want %v", b.PlannedEnd, tt.wantEnd)
			}
		})
	}
}

func TestCarryOver(t *testing.T) {
	target := []Block{
		{ID: "gym", DayID: "next", Title: "gym", Status: Todo, PlannedStart: timeFromStr("14:00"), PlannedEnd: timeFromStr("15:00")},
	}
	tests := []struct {
		name    string
		src     Block
		start   time.Time
		end     time.Time
		wantErr error
	}{
		{name: "todo carries with same times", src: Block{ID: "study", DayID: "day", Title: "study", Status: Todo}, start: timeFromStr("09:00"), end: timeFromStr("10:00")},
		{name: "running carries", src: Block{ID: "code", DayID: "day", Title: "code", Status: InProgress}, start: timeFromStr("09:00"), end: timeFromStr("10:00")},
		{name: "paused carries", src: Block{ID: "code", DayID: "day", Title: "code", Status: Paused}, start: timeFromStr("09:00"), end: timeFromStr("10:00")},
		{name: "done refuses", src: Block{ID: "gym", DayID: "day", Title: "gym", Status: Done}, start: timeFromStr("09:00"), end: timeFromStr("10:00"), wantErr: ErrCarryoverRefused},
		{name: "skipped refuses", src: Block{ID: "read", DayID: "day", Title: "read", Status: Skipped}, start: timeFromStr("09:00"), end: timeFromStr("10:00"), wantErr: ErrCarryoverRefused},
		{name: "archived refuses", src: Block{ID: "old", DayID: "day", Title: "old", Status: Todo, Archived: true}, start: timeFromStr("09:00"), end: timeFromStr("10:00"), wantErr: ErrCarryoverRefused},
		{name: "clash on target refuses", src: Block{ID: "study", DayID: "day", Title: "study", Status: Todo}, start: timeFromStr("14:15"), end: timeFromStr("14:45"), wantErr: ErrOverlap},
		{name: "bad times refuse", src: Block{ID: "study", DayID: "day", Title: "study", Status: Todo}, start: timeFromStr("10:00"), end: timeFromStr("09:00"), wantErr: ErrInvalidStartAndEnd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.src.CarryOver("next", tt.start, tt.end, target)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("got non-nil block, want nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if got.Status != Todo || got.DayID != "next" {
				t.Errorf("got %q on %q, want todo on next", got.Status, got.DayID)
			}
			if got.CopiedFromID == nil || *got.CopiedFromID != tt.src.ID {
				t.Errorf("got copy from %v, want %q", got.CopiedFromID, tt.src.ID)
			}
			if !got.PlannedStart.Equal(tt.start) || !got.PlannedEnd.Equal(tt.end) {
				t.Errorf("got times %v to %v, want %v to %v", got.PlannedStart, got.PlannedEnd, tt.start, tt.end)
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
			now := timeFromStr("12:00")
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

func TestValidate(t *testing.T) {
	now := timeFromStr("09:00")
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
	now := timeFromStr("09:00")
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
