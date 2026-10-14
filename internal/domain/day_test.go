package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/utils"
)

func TestProgress(t *testing.T) {
	mk := func(id string, status Status, archived bool) Block {
		return Block{ID: utils.ID(id), DayID: "day", Title: id, Status: status, Archived: archived}
	}
	tests := []struct {
		name         string
		blocks       []Block
		wantProgress int
		wantOK       bool
	}{
		{name: "mixed day rounds", blocks: []Block{
			mk("a", Done, false), mk("b", Done, false), mk("c", Todo, false),
			mk("d", Skipped, false), mk("e", Done, true),
		}, wantProgress: 67, wantOK: true},
		{name: "all done", blocks: []Block{mk("a", Done, false), mk("b", Done, false)}, wantProgress: 100, wantOK: true},
		{name: "none done", blocks: []Block{mk("a", Todo, false), mk("b", Todo, false)}, wantProgress: 0, wantOK: true},
		{name: "empty day shows dash", blocks: nil, wantProgress: 0, wantOK: false},
		{name: "all skipped shows dash", blocks: []Block{mk("a", Skipped, false)}, wantProgress: 0, wantOK: false},
		{name: "all archived shows dash", blocks: []Block{mk("a", Done, true)}, wantProgress: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Progress(tt.blocks)
			if ok != tt.wantOK || got != tt.wantProgress {
				t.Errorf("got score %d ok %v, want %d ok %v", got, ok, tt.wantProgress, tt.wantOK)
			}
		})
	}
}

func TestNowNext(t *testing.T) {
	mk := func(id string, status Status, sh, sm, eh, em int) Block {
		return Block{ID: utils.ID(id), DayID: "day", Title: id, Status: status,
			PlannedStart: mkTime(sh, sm), PlannedEnd: mkTime(eh, em)}
	}
	active := []Block{
		mk("study", Todo, 9, 0, 10, 0),
		mk("code", InProgress, 10, 30, 11, 30),
		mk("gym", Todo, 14, 0, 15, 0),
	}
	planned := []Block{
		mk("study", Todo, 9, 0, 10, 0),
		mk("code", Todo, 10, 30, 11, 30),
		mk("gym", Todo, 14, 0, 15, 0),
	}
	tests := []struct {
		name     string
		blocks   []Block
		now      time.Time
		wantNow  string
		wantNext string
	}{
		{name: "active wins past its window", blocks: active, now: mkTime(12, 0), wantNow: "code", wantNext: "gym"},
		{name: "planned holds the moment", blocks: planned, now: mkTime(9, 15), wantNow: "study", wantNext: "code"},
		{name: "past everything shows nothing", blocks: planned, now: mkTime(18, 0), wantNow: "", wantNext: ""},
		{name: "empty day shows nothing", blocks: nil, now: mkTime(9, 0), wantNow: "", wantNext: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNow, gotNext := Now(tt.blocks, tt.now), Next(tt.blocks, tt.now)
			if id(gotNow) != tt.wantNow || id(gotNext) != tt.wantNext {
				t.Errorf("got now %q next %q, want %q next %q", id(gotNow), id(gotNext), tt.wantNow, tt.wantNext)
			}
		})
	}
}

func id(b *Block) string {
	if b == nil {
		return ""
	}
	return string(b.ID)
}

func TestNewDay(t *testing.T) {
	tests := []struct {
		name     string
		date     string
		timezone string
		wantErr  error
	}{
		{name: "good day", date: "2026-10-14", timezone: "Asia/Kolkata", wantErr: nil},
		{name: "empty date", date: "", timezone: "Asia/Kolkata", wantErr: ErrInvalidDayDate},
		{name: "wrong date shape", date: "14-10-2026", timezone: "Asia/Kolkata", wantErr: ErrInvalidDayDate},
		{name: "impossible month", date: "2026-13-01", timezone: "Asia/Kolkata", wantErr: ErrInvalidDayDate},
		{name: "empty zone", date: "2026-10-14", timezone: "", wantErr: ErrInvalidDayZone},
		{name: "unknown zone", date: "2026-10-14", timezone: "Earth/Somewhere", wantErr: ErrInvalidDayZone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewDay(tt.date, tt.timezone)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("got non-nil day, want nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if !strings.HasPrefix(string(got.ID), "dy_") {
				t.Errorf("got ID %q, want prefix %q", got.ID, "dy_")
			}
			if got.Date != tt.date || got.Timezone != tt.timezone {
				t.Errorf("got %q in %q, want %q in %q", got.Date, got.Timezone, tt.date, tt.timezone)
			}
			if got.CreatedAt.IsZero() {
				t.Errorf("got zero CreatedAt, want stamp")
			}
		})
	}
}
