package cli

import (
	"context"
	"encoding/json"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
	"testing"
	"time"
)

func TestExtendAndEvents(t *testing.T) {
	f := newCommandFixture(t)
	block := f.saved("add", "09:00", "10:00", "Study")
	f.saved("add", "10:00", "11:00", "Code")
	f.fail("ERR_ILLEGAL_TRANSITION", 1, "extend", "1")
	f.saved("start", "1")
	f.fail("ERR_OVERLAP", 1, "extend", "1")
	if got := f.view().Blocks[0]; got.PlannedEnd.Format("15:04") != "10:00" {
		t.Error("refused extension must retain the planned end")
	}
	f.saved("edit", "--start", "10:15", "--end", "11:15", "2")
	if got := f.saved("extend", "1"); got.PlannedEnd.Format("15:04") != "10:15" {
		t.Error("extension should add exactly fifteen minutes")
	}
	f.saved("delete", "1")
	var events []domain.BlockEvent
	if err := json.Unmarshal([]byte(f.ok("--json", "events", string(block.ID))), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("got %d events, want creation/start/extend/archive", len(events))
	}
	if events[2].Reason != domain.ReasonExtend || events[2].FromStatus != events[2].ToStatus || events[2].Seq <= events[1].Seq {
		t.Error("same-status extension must remain ordered in history")
	}
}

func TestAutomaticPause(t *testing.T) {
	for _, test := range []struct {
		name, command string
		status        domain.Status
		refused       bool
	}{
		{"start switch", "start", domain.Todo, false}, {"resume switch", "resume", domain.Paused, false}, {"reopen switch", "reopen", domain.Done, false}, {"old reopen refused", "reopen", domain.Done, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newCommandFixture(t)
			runner := f.blockFixture("bl_runner", "09:00", "10:00")
			runner.Status = domain.InProgress
			runner.ActualStart = &f.now
			target := f.blockFixture("bl_target", "11:00", "12:00")
			target.Status = test.status
			if test.status == domain.Done {
				target.ActualStart = &f.now
				target.ActualEnd = &f.now
			}
			f.seed("2026-10-14", "UTC", runner, target)
			if test.refused {
				f.now = f.now.AddDate(0, 0, 1)
				f.fail("ERR_ILLEGAL_TRANSITION", 1, "--date", "2026-10-14", test.command, "2")
			} else {
				f.saved(test.command, "2")
			}
			db, err := store.Open(context.Background(), f.path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			saved, err := db.BlockByID(context.Background(), runner.ID)
			if err != nil {
				t.Fatal(err)
			}
			events, err := db.EventsByBlock(context.Background(), runner.ID)
			if err != nil {
				t.Fatal(err)
			}
			targets, err := db.EventsByBlock(context.Background(), target.ID)
			if err != nil {
				t.Fatal(err)
			}
			if test.refused {
				if saved.Status != domain.InProgress || len(events) != 1 || len(targets) != 1 {
					t.Error("refused target must not pause or append either history")
				}
				return
			}
			if saved.Status != domain.Paused || len(events) != 2 || len(targets) != 2 {
				t.Fatal("switch should save both changes and histories")
			}
			if events[1].Reason != domain.ReasonAutoPause || !events[1].OccurredAt.Equal(targets[1].OccurredAt) || !events[1].OccurredAt.Equal(f.now) {
				t.Error("atomic switch needs auto-pause reason and one command instant")
			}
		})
	}
}

func TestLifecycleCommands(t *testing.T) {
	for _, test := range []struct {
		name, command string
		from, to      domain.Status
		refused       bool
	}{
		{"start todo", "start", domain.Todo, domain.InProgress, false},
		{"pause runner", "pause", domain.InProgress, domain.Paused, false},
		{"resume paused", "resume", domain.Paused, domain.InProgress, false},
		{"finish runner", "finish", domain.InProgress, domain.Done, false},
		{"finish paused", "finish", domain.Paused, domain.Done, false},
		{"skip todo", "skip", domain.Todo, domain.Skipped, false},
		{"skip runner", "skip", domain.InProgress, domain.Skipped, false},
		{"skip paused", "skip", domain.Paused, domain.Skipped, false},
		{"reopen today", "reopen", domain.Done, domain.InProgress, false},
		{"start paused refused", "start", domain.Paused, domain.InProgress, true},
		{"pause todo refused", "pause", domain.Todo, domain.Paused, true},
		{"resume todo refused", "resume", domain.Todo, domain.InProgress, true},
		{"finish todo refused", "finish", domain.Todo, domain.Done, true},
		{"skip done refused", "skip", domain.Done, domain.Skipped, true},
		{"reopen skipped refused", "reopen", domain.Skipped, domain.InProgress, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newCommandFixture(t)
			block := f.blockFixture("bl_work", "09:00", "10:00")
			block.Status = test.from
			earlier := f.now.Add(-10 * time.Minute)
			if test.from != domain.Todo {
				block.ActualStart = &earlier
			}
			if test.from == domain.Done {
				block.ActualEnd = &earlier
			}
			f.seed("2026-10-14", "UTC", block)
			if test.refused {
				f.fail("ERR_ILLEGAL_TRANSITION", 1, test.command, "1")
			} else {
				saved := f.saved(test.command, "1")
				if saved.Status != test.to {
					t.Errorf("got status %s, want %s", saved.Status, test.to)
				}
				if block.ActualStart != nil && (saved.ActualStart == nil || !saved.ActualStart.Equal(*block.ActualStart)) {
					t.Error("existing actual start must be retained")
				}
				if test.command == "reopen" && saved.ActualEnd != nil {
					t.Error("reopen must clear actual end")
				}
			}
			db, err := store.Open(context.Background(), f.path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			events, err := db.EventsByBlock(context.Background(), block.ID)
			if err != nil {
				t.Fatal(err)
			}
			count := 2
			if test.refused {
				count = 1
			}
			if len(events) != count {
				t.Fatalf("got %d events, want %d", len(events), count)
			}
			if !test.refused && (events[1].FromStatus != test.from || events[1].ToStatus != test.to || !events[1].OccurredAt.Equal(f.now)) {
				t.Error("event must use exact source/target and captured command time")
			}
		})
	}
}
