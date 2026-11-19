package store

import (
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func TestSettleDueClosesEndedDaysOnce(t *testing.T) {
	s := openTest(t)
	day, running := createTestItem(t, s)
	done, _ := domain.NewItem(day.ID, "Done", 30, clock("08:10:00"))
	_ = s.Create(ctx, nil, Change{Item: done, Event: domain.Record(nil, done, done.CreatedAt, done.CreatedAt, domain.ReasonUser)})
	done = act(t, s, day, done, domain.Start, clock("09:00:00"))
	done = act(t, s, day, done, domain.Finish, clock("09:30:00"))
	running = act(t, s, day, running, domain.Start, clock("10:00:00"))

	if err := s.SettleDue(ctx, clock("23:00:00")); err != nil {
		t.Fatal(err)
	}
	if saved, _ := s.Item(ctx, running.ID); saved.Status != domain.InProgress {
		t.Fatal("an open day must not close")
	}

	noticed := clock("23:59:59").Add(9 * time.Hour)
	for i := 0; i < 2; i++ {
		if err := s.SettleDue(ctx, noticed); err != nil {
			t.Fatal(err)
		}
	}
	closed, _ := s.Item(ctx, running.ID)
	midnight := clock("23:59:59").Add(time.Second)
	if closed.Status != domain.Incomplete || !closed.ClosedAt.Equal(midnight) || closed.FinishedAt != nil {
		t.Fatalf("the running item was not closed at midnight: %+v", closed)
	}
	events, _ := s.Events(ctx, running.ID)
	if len(events) != 3 || events[2].Reason != domain.ReasonDayEnd {
		t.Fatalf("expected exactly one day end event, got %d", len(events))
	}
	if saved, _ := s.Item(ctx, done.ID); saved.Status != domain.Done {
		t.Fatal("a done item must stay done")
	}
	m, err := domain.Measure(day, closed, events, noticed)
	if err != nil || m.WorkNS != int64(14*time.Hour) {
		t.Fatalf("work %v, %v", time.Duration(m.WorkNS), err)
	}
}

func TestSettleDueRespectsEachDaysTimezone(t *testing.T) {
	s := openTest(t)
	day, err := domain.NewDay("2026-11-17", "Asia/Kolkata", clock("00:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	item, _ := domain.NewItem(day.ID, "Study", 30, clock("01:00:00"))
	if err := s.Create(ctx, &day, Change{Item: item, Event: domain.Record(nil, item, item.CreatedAt, item.CreatedAt, domain.ReasonUser)}); err != nil {
		t.Fatal(err)
	}
	// Midnight in India is 18:30 UTC.
	_ = s.SettleDue(ctx, clock("18:29:59"))
	if saved, _ := s.Item(ctx, item.ID); saved.Status != domain.Todo {
		t.Fatal("closed before midnight in India")
	}
	_ = s.SettleDue(ctx, clock("18:30:00"))
	if saved, _ := s.Item(ctx, item.ID); saved.Status != domain.Incomplete {
		t.Fatal("not closed at midnight in India")
	}
}
