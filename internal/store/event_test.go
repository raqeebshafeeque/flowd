package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"modernc.org/sqlite"
)

func TestCreateBlockWithHistory(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		name := "complete new day"
		if invalid {
			name = "event failure leaves no new day"
		}
		t.Run(name, func(t *testing.T) {
			store := openTestStore(t)
			day := exampleDay(t)
			block := exampleBlock(t, "bl_study", "09:00", "10:00")
			event := exampleEvent(block, domain.Todo, block.CreatedAt, domain.ReasonUser)
			if invalid {
				event.Reason = "unknown"
			}
			err := store.CreateDayWithBlock(context.Background(), day, block, event)
			if invalid {
				var cause *sqlite.Error
				if !errors.As(err, &cause) {
					t.Fatalf("want event constraint cause, got %v", err)
				}
				if _, err := store.DayByID(context.Background(), day.ID); !errors.Is(err, ErrDayNotFound) {
					t.Errorf("partial day survived: %v", err)
				}
				if _, err := store.BlockByID(context.Background(), block.ID); !errors.Is(err, ErrBlockNotFound) {
					t.Errorf("partial block survived: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				got, err := store.BlockByID(context.Background(), block.ID)
				if err != nil {
					t.Fatal(err)
				}
				assertBlock(t, got, block)
			}
			var count int
			if err := store.db.QueryRow("SELECT count(*) FROM block_events").Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 1
			if invalid {
				want = 0
			}
			if count != want {
				t.Errorf("got %d events, want %d", count, want)
			}
		})
	}
}

func TestCreateBlockOnSavedDay(t *testing.T) {
	store := blockTestStore(t)
	block := exampleBlock(t, "bl_study", "09:00", "10:00")
	if err := store.CreateBlock(context.Background(), block, exampleEvent(block, domain.Todo, block.CreatedAt, domain.ReasonUser)); err != nil {
		t.Fatal(err)
	}
}

func TestSaveMovesAndHistory(t *testing.T) {
	store := blockTestStore(t)
	ctx := context.Background()
	first := exampleBlock(t, "bl_first", "09:00", "10:00")
	second := exampleBlock(t, "bl_second", "10:00", "11:00")
	for _, block := range []domain.Block{first, second} {
		if err := store.CreateBlock(ctx, block, exampleEvent(block, block.Status, block.CreatedAt, domain.ReasonUser)); err != nil {
			t.Fatal(err)
		}
	}
	stamp := testTimestamp(t, "2026-10-14T09:02:00Z")
	first.Status = domain.InProgress
	first.ActualStart = &stamp
	if err := store.SaveMoves(ctx, []BlockChange{{Block: first, Event: exampleEvent(first, domain.Todo, stamp, domain.ReasonUser)}}); err != nil {
		t.Fatal(err)
	}
	first.Status = domain.Paused
	second.Status = domain.InProgress
	second.ActualStart = &stamp
	if err := store.SaveMoves(ctx, []BlockChange{
		{Block: first, Event: exampleEvent(first, domain.InProgress, stamp, domain.ReasonAutoPause)},
		{Block: second, Event: exampleEvent(second, domain.Todo, stamp, domain.ReasonUser)},
	}); err != nil {
		t.Fatal(err)
	}
	first.Title = "Edited"
	if err := store.SaveMoves(ctx, []BlockChange{{Block: first, Event: exampleEvent(first, domain.Paused, stamp, domain.ReasonUser)}}); err != nil {
		t.Fatal(err)
	}
	first.UpdatedAt = &stamp
	got, err := store.BlockByID(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertBlock(t, got, first)
	events, err := store.EventsByBlock(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4", len(events))
	}
	for index, event := range events {
		if index > 0 && event.Seq <= events[index-1].Seq {
			t.Error("sequence numbers must increase")
		}
	}
	if events[2].Reason != domain.ReasonAutoPause || events[3].FromStatus != events[3].ToStatus {
		t.Error("history should preserve auto-pause and same-status edits")
	}
}

func TestInvalidChanges(t *testing.T) {
	for _, kind := range []string{"event block id", "target status", "source status", "creation time", "day id", "copy source", "sequence supplied", "zero event time", "duplicate block", "archived target"} {
		t.Run(kind, func(t *testing.T) {
			store := blockTestStore(t)
			ctx := context.Background()
			original := exampleBlock(t, "bl_original", "09:00", "10:00")
			original.Archived = kind == "archived target"
			other := exampleBlock(t, "bl_other", "10:00", "11:00")
			for _, block := range []domain.Block{original, other} {
				if err := store.CreateBlock(ctx, block, exampleEvent(block, block.Status, block.CreatedAt, domain.ReasonUser)); err != nil {
					t.Fatal(err)
				}
			}
			otherDay := exampleDay(t)
			otherDay.ID = "dy_other"
			otherDay.Date = "2026-10-15"
			if err := store.CreateDay(ctx, otherDay); err != nil {
				t.Fatal(err)
			}
			changed := original
			changed.Title = "Edited"
			event := exampleEvent(changed, domain.Todo, changed.CreatedAt.Add(time.Hour), domain.ReasonUser)
			switch kind {
			case "event block id":
				event.BlockID = other.ID
			case "target status":
				event.ToStatus = domain.Done
			case "source status":
				event.FromStatus = domain.Paused
			case "creation time":
				changed.CreatedAt = changed.CreatedAt.Add(time.Nanosecond)
			case "day id":
				changed.DayID = otherDay.ID
			case "copy source":
				changed.CopiedFromID = &other.ID
			case "sequence supplied":
				event.Seq = 42
			case "zero event time":
				event.OccurredAt = time.Time{}
			}
			changes := []BlockChange{{Block: changed, Event: event}}
			if kind == "duplicate block" {
				changes = append(changes, changes[0])
			}
			if err := store.SaveMoves(ctx, changes); err == nil {
				t.Error("invalid change should be refused")
			}
			got, err := store.BlockByID(ctx, original.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertBlock(t, got, original)
			events, err := store.EventsByBlock(ctx, original.ID)
			if err != nil || len(events) != 1 {
				t.Errorf("want original history, got %d events and %v", len(events), err)
			}
		})
	}
}

func TestInvalidInitialEvent(t *testing.T) {
	store := blockTestStore(t)
	ctx := context.Background()
	existing := exampleBlock(t, "bl_existing", "07:00", "08:00")
	if err := store.CreateBlock(ctx, existing, exampleEvent(existing, existing.Status, existing.CreatedAt, domain.ReasonUser)); err != nil {
		t.Fatal(err)
	}
	candidate := exampleBlock(t, "bl_new", "09:00", "10:00")
	event := exampleEvent(candidate, candidate.Status, candidate.CreatedAt, domain.ReasonUser)
	event.BlockID = existing.ID
	if err := store.CreateBlock(ctx, candidate, event); err == nil {
		t.Error("initial event must match the new block")
	}
	if _, err := store.BlockByID(ctx, candidate.ID); !errors.Is(err, ErrBlockNotFound) {
		t.Errorf("invalid creation must roll back, got %v", err)
	}
}

func TestPlanConsistency(t *testing.T) {
	for _, test := range []struct {
		name            string
		status          domain.Status
		archived        bool
		start, end      string
		candidateStatus domain.Status
		rejected        bool
	}{
		{"touching intervals", domain.Todo, false, "10:00", "11:00", domain.Todo, false},
		{"overlap", domain.Todo, false, "09:30", "10:30", domain.Todo, true},
		{"skipped slot", domain.Skipped, false, "09:30", "10:30", domain.Todo, true},
		{"archived slot", domain.Todo, true, "09:30", "10:30", domain.Todo, false},
		{"two runners", domain.InProgress, false, "10:00", "11:00", domain.InProgress, true},
		{"invalid title", domain.Todo, false, "10:00", "11:00", domain.Todo, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := blockTestStore(t)
			ctx := context.Background()
			original := exampleBlock(t, "bl_original", "09:00", "10:00")
			original.Status, original.Archived = test.status, test.archived
			if err := store.CreateBlock(ctx, original, exampleEvent(original, original.Status, original.CreatedAt, domain.ReasonUser)); err != nil {
				t.Fatal(err)
			}
			candidate := exampleBlock(t, "bl_candidate", test.start, test.end)
			candidate.Status = test.candidateStatus
			if test.name == "invalid title" {
				candidate.Title = ""
			}
			err := store.CreateBlock(ctx, candidate, exampleEvent(candidate, candidate.Status, candidate.CreatedAt, domain.ReasonUser))
			if (err != nil) != test.rejected {
				t.Fatalf("got error %v, rejected should be %v", err, test.rejected)
			}
			if test.rejected {
				if _, err := store.BlockByID(ctx, candidate.ID); !errors.Is(err, ErrBlockNotFound) {
					t.Errorf("rejected block persisted: %v", err)
				}
			}
		})
	}
}

func TestPlanConsistencyOnUpdate(t *testing.T) {
	store := blockTestStore(t)
	ctx := context.Background()
	first := exampleBlock(t, "bl_first", "09:00", "10:00")
	second := exampleBlock(t, "bl_second", "10:00", "11:00")
	for _, block := range []domain.Block{first, second} {
		if err := store.CreateBlock(ctx, block, exampleEvent(block, block.Status, block.CreatedAt, domain.ReasonUser)); err != nil {
			t.Fatal(err)
		}
	}
	changed := second
	changed.PlannedStart = first.PlannedStart.Add(time.Minute)
	err := store.SaveMoves(ctx, []BlockChange{{Block: changed, Event: exampleEvent(changed, second.Status, second.CreatedAt, domain.ReasonUser)}})
	if !errors.Is(err, domain.ErrOverlap) {
		t.Fatalf("want overlap, got %v", err)
	}
	got, err := store.BlockByID(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertBlock(t, got, second)
	events, err := store.EventsByBlock(ctx, second.ID)
	if err != nil || len(events) != 1 {
		t.Errorf("want unchanged history, got %d events and %v", len(events), err)
	}
}

func TestGroupedRollback(t *testing.T) {
	for _, kind := range []string{"bad second event", "missing second block", "stale second status", "overlap", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			store := blockTestStore(t)
			ctx := context.Background()
			first := exampleBlock(t, "bl_first", "09:00", "10:00")
			first.Status = domain.InProgress
			second := exampleBlock(t, "bl_second", "10:00", "11:00")
			for _, block := range []domain.Block{first, second} {
				if err := store.CreateBlock(ctx, block, exampleEvent(block, block.Status, block.CreatedAt, domain.ReasonUser)); err != nil {
					t.Fatal(err)
				}
			}
			paused, running := first, second
			paused.Status = domain.Paused
			running.Status = domain.InProgress
			stamp := testTimestamp(t, "2026-10-14T10:15:00Z")
			running.ActualStart = &stamp
			changes := []BlockChange{
				{Block: paused, Event: exampleEvent(paused, domain.InProgress, stamp, domain.ReasonAutoPause)},
				{Block: running, Event: exampleEvent(running, domain.Todo, stamp, domain.ReasonUser)},
			}
			switch kind {
			case "bad second event":
				changes[1].Event.Reason = "unknown"
			case "missing second block":
				changes[1].Block.ID = "bl_missing"
				changes[1].Event.BlockID = "bl_missing"
			case "stale second status":
				changes[1].Event.FromStatus = domain.Done
			case "overlap":
				changes[1].Block.PlannedStart = first.PlannedStart.Add(time.Minute)
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if err := store.SaveMoves(ctx, changes); err == nil {
				t.Fatal("bad group should fail")
			}
			for _, original := range []domain.Block{first, second} {
				got, err := store.BlockByID(context.Background(), original.ID)
				if err != nil {
					t.Fatal(err)
				}
				assertBlock(t, got, original)
				events, err := store.EventsByBlock(context.Background(), original.ID)
				if err != nil || len(events) != 1 {
					t.Errorf("partial history survived: %d events, %v", len(events), err)
				}
			}
			if store.db.Stats().InUse != 0 {
				t.Error("failed batch should release its transaction")
			}
		})
	}
}

func TestGroupedFinalPlan(t *testing.T) {
	store := blockTestStore(t)
	ctx := context.Background()
	first := exampleBlock(t, "bl_first", "09:00", "10:00")
	first.Status = domain.InProgress
	second := exampleBlock(t, "bl_second", "10:00", "11:00")
	for _, block := range []domain.Block{first, second} {
		if err := store.CreateBlock(ctx, block, exampleEvent(block, block.Status, block.CreatedAt, domain.ReasonUser)); err != nil {
			t.Fatal(err)
		}
	}
	first.Status = domain.Paused
	second.Status = domain.InProgress
	stamp := second.PlannedStart
	if err := store.SaveMoves(ctx, []BlockChange{
		{Block: second, Event: exampleEvent(second, domain.Todo, stamp, domain.ReasonUser)},
		{Block: first, Event: exampleEvent(first, domain.InProgress, stamp, domain.ReasonAutoPause)},
	}); err != nil {
		t.Fatalf("only the final plan should be validated: %v", err)
	}
}

func TestArchiveBlock(t *testing.T) {
	store := blockTestStore(t)
	ctx := context.Background()
	original := exampleBlock(t, "bl_original", "09:00", "10:00")
	original.Status = domain.InProgress
	started := original.PlannedStart
	original.ActualStart = &started
	if err := store.CreateBlock(ctx, original, exampleEvent(original, original.Status, original.CreatedAt, domain.ReasonUser)); err != nil {
		t.Fatal(err)
	}
	stamp := started.Add(time.Minute)
	for repeat := 0; repeat < 2; repeat++ {
		if err := store.ArchiveBlock(ctx, original.ID, stamp); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.BlockByID(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := original
	want.Archived = true
	want.UpdatedAt = &stamp
	assertBlock(t, got, want)
	blocks, err := store.BlocksByDay(ctx, original.DayID)
	if err != nil || len(blocks) != 0 {
		t.Errorf("archived block should be hidden: %v", err)
	}
	events, err := store.EventsByBlock(ctx, original.ID)
	if err != nil || len(events) != 2 {
		t.Fatalf("want one archive event, got %d events and %v", len(events), err)
	}
	if events[1].FromStatus != original.Status || events[1].ToStatus != original.Status {
		t.Error("archive should keep status")
	}
	if err := store.ArchiveBlock(ctx, "bl_missing", stamp); !errors.Is(err, ErrBlockNotFound) {
		t.Errorf("want not found, got %v", err)
	}
}

func exampleEvent(block domain.Block, from domain.Status, stamp time.Time, reason domain.EventReason) domain.BlockEvent {
	return domain.BlockEvent{BlockID: block.ID, FromStatus: from, ToStatus: block.Status, OccurredAt: stamp, Reason: reason}
}
