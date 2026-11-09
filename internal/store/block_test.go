package store

import (
	"context"
	"errors"
	"strings"

	"modernc.org/sqlite"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/utils"
)

func TestBlockMapping(t *testing.T) {
	for _, test := range []struct {
		name                string
		populated, archived bool
	}{
		{"optional fields absent", false, false}, {"all fields populated", true, false}, {"archived row", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := blockTestStore(t)
			block := exampleBlock(t, "bl_study", "09:00", "10:00")
			block.Archived = test.archived
			if test.populated {
				source := exampleBlock(t, "bl_source", "07:00", "08:00")
				saveBlockFixture(t, store, source)
				block.CopiedFromID = &source.ID
				started, ended := block.PlannedStart.Add(time.Minute), block.PlannedEnd.Add(time.Minute)
				block.ActualStart, block.ActualEnd, block.UpdatedAt = &started, &ended, &ended
				block.Status = domain.Done
			}
			saveBlockFixture(t, store, block)
			got, err := scanBlock(store.db.QueryRow("SELECT "+blockColumns+" FROM blocks WHERE id = ?", block.ID))
			if err != nil {
				t.Fatal(err)
			}
			assertBlock(t, got, block)
		})
	}
}

func TestBlockLookupAndConstraints(t *testing.T) {
	for _, kind := range []string{"archived lookup", "duplicate id", "missing day", "missing source"} {
		t.Run(kind, func(t *testing.T) {
			store := blockTestStore(t)
			original := exampleBlock(t, "bl_original", "09:00", "10:00")
			original.Archived = true
			saveBlockFixture(t, store, original)
			got, err := store.BlockByID(context.Background(), original.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertBlock(t, got, original)
			if _, err := store.BlockByID(context.Background(), "bl_missing"); !errors.Is(err, ErrBlockNotFound) {
				t.Fatalf("want not found, got %v", err)
			}
			if kind == "archived lookup" {
				return
			}
			candidate := exampleBlock(t, "bl_candidate", "10:00", "11:00")
			switch kind {
			case "duplicate id":
				candidate.ID = original.ID
			case "missing day":
				candidate.DayID = "dy_missing"
			case "missing source":
				missing := utils.ID("bl_missing")
				candidate.CopiedFromID = &missing
			}
			tx, err := store.db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			err = insertBlock(context.Background(), tx, candidate)
			var cause *sqlite.Error
			if !errors.As(err, &cause) {
				t.Errorf("want SQLite constraint cause, got %v", err)
			}
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			got, err = store.BlockByID(context.Background(), original.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertBlock(t, got, original)
		})
	}
}

func TestBlocksByDay(t *testing.T) {
	store := blockTestStore(t)
	ctx := context.Background()
	empty, err := store.BlocksByDay(ctx, "dy_test")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("want empty collection, got length %d and %v", len(empty), err)
	}
	first := exampleBlock(t, "bl_z_first", "09:00", "10:00")
	first.PlannedEnd = first.PlannedStart.Add(50 * time.Nanosecond)
	second := exampleBlock(t, "bl_a_second", "09:00", "10:00")
	second.PlannedStart = first.PlannedStart.Add(100 * time.Nanosecond)
	second.PlannedEnd = first.PlannedStart.Add(150 * time.Nanosecond)
	archived := exampleBlock(t, "bl_archived", "08:00", "09:00")
	archived.Archived = true
	otherDay := exampleDay(t)
	otherDay.ID = "dy_other"
	otherDay.Date = "2026-10-15"
	if err := store.CreateDay(ctx, otherDay); err != nil {
		t.Fatal(err)
	}
	other := exampleBlock(t, "bl_other", "08:00", "09:00")
	other.DayID = otherDay.ID
	for _, block := range []domain.Block{second, archived, other, first} {
		saveBlockFixture(t, store, block)
	}
	blocks, err := store.BlocksByDay(ctx, "dy_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d visible blocks, want 2", len(blocks))
	}
	assertBlock(t, &blocks[0], first)
	assertBlock(t, &blocks[1], second)
	if _, err := store.BlocksByDay(ctx, "dy_missing"); !errors.Is(err, ErrDayNotFound) {
		t.Errorf("want day not found, got %v", err)
	}
}

func TestBlockMalformedTimes(t *testing.T) {
	for _, field := range []struct{ column, name string }{
		{"planned_start_utc", "planned start"}, {"planned_end_utc", "planned end"},
		{"created_at_utc", "creation time"}, {"actual_start_utc", "actual start"},
		{"actual_end_utc", "actual end"}, {"updated_at_utc", "update time"},
	} {
		t.Run(field.name, func(t *testing.T) {
			store := blockTestStore(t)
			block := exampleBlock(t, "bl_test", "09:00", "10:00")
			saveBlockFixture(t, store, block)
			if _, err := store.db.Exec("UPDATE blocks SET " + field.column + " = 'invalid'"); err != nil {
				t.Fatal(err)
			}
			_, err := store.BlockByID(context.Background(), block.ID)
			var cause *time.ParseError
			if !errors.As(err, &cause) || !strings.Contains(err.Error(), field.name) {
				t.Errorf("want %s context and parse cause, got %v", field.name, err)
			}
			if _, err := store.BlocksByDay(context.Background(), block.DayID); !errors.As(err, &cause) {
				t.Errorf("list should report parsing cause, got %v", err)
			}
		})
	}
}

func TestBlockUTCAndCancellation(t *testing.T) {
	store := blockTestStore(t)
	block := exampleBlock(t, "bl_'; DROP TABLE blocks; --", "09:00", "10:00")
	block.Title = "read Go's notes; --"
	block.PlannedStart = testTimestamp(t, "2026-10-14T09:00:00.123456789+05:30")
	block.PlannedEnd = block.PlannedStart.Add(time.Hour)
	block.CreatedAt = block.PlannedStart.Add(-time.Hour)
	started, ended := block.PlannedStart.Add(time.Minute), block.PlannedEnd.Add(time.Minute)
	block.ActualStart, block.ActualEnd, block.UpdatedAt = &started, &ended, &ended
	saveBlockFixture(t, store, block)
	got, err := store.BlockByID(context.Background(), block.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertBlock(t, got, block)
	if got.PlannedStart.Location() != time.UTC || got.ActualStart == nil || got.ActualStart.Location() != time.UTC {
		t.Error("saved times should normalize to UTC")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.BlockByID(ctx, block.ID); !errors.Is(err, context.Canceled) {
		t.Errorf("want cancellation, got %v", err)
	}
	if _, err := store.BlocksByDay(ctx, block.DayID); !errors.Is(err, context.Canceled) {
		t.Errorf("want cancellation, got %v", err)
	}
}

func blockTestStore(t *testing.T) *Store {
	t.Helper()
	store := openTestStore(t)
	if err := store.CreateDay(context.Background(), exampleDay(t)); err != nil {
		t.Fatal(err)
	}
	return store
}

func exampleBlock(t *testing.T, id utils.ID, start, end string) domain.Block {
	t.Helper()
	return domain.Block{ID: id, DayID: "dy_test", Title: string(id), Status: domain.Todo,
		CreatedAt:    testTimestamp(t, "2026-10-14T08:00:00Z"),
		PlannedStart: testTimestamp(t, "2026-10-14T"+start+":00Z"),
		PlannedEnd:   testTimestamp(t, "2026-10-14T"+end+":00Z")}
}

// Fixtures can insert directly; public mutations will pair changes with events.
func saveBlockFixture(t *testing.T, store *Store, block domain.Block) {
	t.Helper()
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := insertBlock(context.Background(), tx, block); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func assertBlock(t *testing.T, got *domain.Block, want domain.Block) {
	t.Helper()
	if got == nil {
		t.Fatal("want saved block, got nil")
	}
	for _, field := range []struct{ name, got, want string }{
		{"id", string(got.ID), string(want.ID)}, {"day id", string(got.DayID), string(want.DayID)},
		{"title", got.Title, want.Title}, {"status", string(got.Status), string(want.Status)},
	} {
		if field.got != field.want {
			t.Errorf("got %s %q, want %q", field.name, field.got, field.want)
		}
	}
	if got.Archived != want.Archived {
		t.Errorf("got archived %v, want %v", got.Archived, want.Archived)
	}
	assertTime(t, "creation time", &got.CreatedAt, &want.CreatedAt)
	assertTime(t, "planned start", &got.PlannedStart, &want.PlannedStart)
	assertTime(t, "planned end", &got.PlannedEnd, &want.PlannedEnd)
	assertTime(t, "actual start", got.ActualStart, want.ActualStart)
	assertTime(t, "actual end", got.ActualEnd, want.ActualEnd)
	assertTime(t, "update time", got.UpdatedAt, want.UpdatedAt)
	if got.CopiedFromID == nil || want.CopiedFromID == nil {
		if (got.CopiedFromID == nil) != (want.CopiedFromID == nil) {
			t.Error("missing copy source differs")
		}
	} else if *got.CopiedFromID != *want.CopiedFromID {
		t.Errorf("got copy source %q, want %q", *got.CopiedFromID, *want.CopiedFromID)
	}
}
