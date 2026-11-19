package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

var ctx = context.Background()

// clock returns 2026-11-17 at the given time in UTC.
func clock(hhmmss string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", "2026-11-17 "+hhmmss)
	if err != nil {
		panic(err)
	}
	return t
}

func createTestItem(t *testing.T, s *Store) (domain.Day, domain.Item) {
	t.Helper()
	day, err := domain.NewDay("2026-11-17", "UTC", clock("08:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	item, err := domain.NewItem(day.ID, "Study", 60, clock("08:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, &day, Change{Item: item, Event: domain.Record(nil, item, item.CreatedAt, item.CreatedAt, domain.ReasonUser)}); err != nil {
		t.Fatal(err)
	}
	return day, item
}

// act applies an action and saves it, returning the saved item.
func act(t *testing.T, s *Store, day domain.Day, item domain.Item, a domain.Action, at time.Time) domain.Item {
	t.Helper()
	before := item.Clone()
	if err := item.Apply(a, at, day); err != nil {
		t.Fatal(err)
	}
	reason := domain.ReasonUser
	if a == domain.Reopen {
		reason = domain.ReasonReopen
	}
	if err := s.Save(ctx, Change{item, domain.Record(&before, item, at, at, reason)}); err != nil {
		t.Fatal(err)
	}
	return item
}

func TestCreateAndReadBack(t *testing.T) {
	s := openTest(t)
	day, item := createTestItem(t, s)
	savedDay, err := s.DayByDate(ctx, "2026-11-17")
	if err != nil || savedDay.ID != day.ID || savedDay.Timezone != "UTC" || !savedDay.CreatedAt.Equal(day.CreatedAt) {
		t.Fatalf("day: %+v, %v", savedDay, err)
	}
	saved, err := s.Item(ctx, item.ID)
	if err != nil || !domain.Same(saved, item) {
		t.Fatalf("item: %+v, %v", saved, err)
	}
	items, err := s.Items(ctx, day.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("items: %v, %v", items, err)
	}
	if _, err := s.DayByDate(ctx, "2026-11-18"); !errors.Is(err, ErrNotFound) {
		t.Fatal("a missing day must be ErrNotFound", err)
	}
	if _, err := s.Item(ctx, "it_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal("a missing item must be ErrNotFound", err)
	}
}

func TestChangesAndHistorySurviveReopening(t *testing.T) {
	path := t.TempDir() + "/flowd.sqlite"
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	day, item := createTestItem(t, s)
	item = act(t, s, day, item, domain.Start, clock("09:00:00"))
	item = act(t, s, day, item, domain.Pause, clock("09:10:00"))
	s.Close()

	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	saved, err := s.Item(ctx, item.ID)
	if err != nil || !domain.Same(saved, item) {
		t.Fatalf("reopened item differs: %+v %v", saved, err)
	}
	events, err := s.Events(ctx, item.ID)
	if err != nil || len(events) != 3 {
		t.Fatalf("got %d events, %v", len(events), err)
	}
	m, err := domain.Measure(day, saved, events, clock("12:00:00"))
	if err != nil || m.WorkNS != int64(10*time.Minute) {
		t.Fatalf("work %v, %v", time.Duration(m.WorkNS), err)
	}
	byDay, err := s.DayEvents(ctx, day.ID)
	if err != nil || len(byDay[item.ID]) != 3 {
		t.Fatalf("day events: %v, %v", byDay, err)
	}
}

func TestStaleChangesAreRefused(t *testing.T) {
	s := openTest(t)
	day, item := createTestItem(t, s)
	old := item.Clone()
	act(t, s, day, item, domain.Start, clock("09:00:00"))
	before := old.Clone()
	_ = old.Apply(domain.Skip, clock("09:05:00"), day)
	if err := s.Save(ctx, Change{old, domain.Record(&before, old, clock("09:05:00"), clock("09:05:00"), domain.ReasonUser)}); !errors.Is(err, ErrStale) {
		t.Fatal("a change based on an old read must be refused", err)
	}
}

func TestInvalidSavesChangeNothing(t *testing.T) {
	s := openTest(t)
	day, item := createTestItem(t, s)
	before := item.Clone()
	started := item.Clone()
	_ = started.Apply(domain.Start, clock("09:00:00"), day)

	other := started.Clone()
	other.Title = "Different"
	cases := map[string]Change{
		"item and event disagree": {other, domain.Record(&before, started, clock("09:00:00"), clock("09:00:00"), domain.ReasonUser)},
		"no before state":         {started, domain.Record(nil, started, clock("09:00:00"), clock("09:00:00"), domain.ReasonUser)},
	}
	for name, c := range cases {
		if err := s.Save(ctx, c); !errors.Is(err, ErrInvalidSave) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	forged := started.Clone()
	forged.Status = domain.Done
	if err := s.Save(ctx, Change{forged, domain.Record(&before, forged, clock("09:00:00"), clock("09:00:00"), domain.ReasonUser)}); err == nil {
		t.Error("a forged event was saved")
	}
	if saved, _ := s.Item(ctx, item.ID); !domain.Same(saved, item) {
		t.Fatal("a refused save changed the item")
	}
}

func TestTwoRunningItemsAreRefused(t *testing.T) {
	s := openTest(t)
	day, a := createTestItem(t, s)
	b, _ := domain.NewItem(day.ID, "Code", 30, clock("08:30:00"))
	if err := s.Create(ctx, nil, Change{Item: b, Event: domain.Record(nil, b, b.CreatedAt, b.CreatedAt, domain.ReasonUser)}); err != nil {
		t.Fatal(err)
	}
	act(t, s, day, a, domain.Start, clock("09:00:00"))
	before := b.Clone()
	_ = b.Apply(domain.Start, clock("09:05:00"), day)
	if err := s.Save(ctx, Change{b, domain.Record(&before, b, clock("09:05:00"), clock("09:05:00"), domain.ReasonUser)}); !errors.Is(err, domain.ErrAlreadyRunning) {
		t.Fatal("a second runner must be refused", err)
	}
}

func TestClockGoingBackwardsIsRefused(t *testing.T) {
	s := openTest(t)
	day, a := createTestItem(t, s)
	b, _ := domain.NewItem(day.ID, "Code", 30, clock("08:30:00"))
	_ = s.Create(ctx, nil, Change{Item: b, Event: domain.Record(nil, b, b.CreatedAt, b.CreatedAt, domain.ReasonUser)})
	act(t, s, day, a, domain.Start, clock("10:00:00"))
	before := b.Clone()
	_ = b.Apply(domain.Skip, clock("09:00:00"), day)
	if err := s.Save(ctx, Change{b, domain.Record(&before, b, clock("09:00:00"), clock("09:00:00"), domain.ReasonUser)}); !errors.Is(err, domain.ErrInvalidEvidence) {
		t.Fatal("a change earlier than the day's last change must be refused", err)
	}
}

func TestArchivedItemsAreHidden(t *testing.T) {
	s := openTest(t)
	day, item := createTestItem(t, s)
	before := item.Clone()
	_ = item.Archive(clock("09:00:00"))
	if err := s.Save(ctx, Change{item, domain.Record(&before, item, clock("09:00:00"), clock("09:00:00"), domain.ReasonUser)}); err != nil {
		t.Fatal(err)
	}
	if items, _ := s.Items(ctx, day.ID); len(items) != 0 {
		t.Fatal("an archived item is still listed")
	}
	if saved, err := s.Item(ctx, item.ID); err != nil || !saved.Archived {
		t.Fatal("the archived item must still be readable by id", err)
	}
}

func TestDamagedStoredTimeIsReported(t *testing.T) {
	s := openTest(t)
	_, item := createTestItem(t, s)
	if _, err := s.db.Exec("UPDATE items SET created_at='yesterday' WHERE id=?", item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Item(ctx, item.ID); !errors.Is(err, ErrCorrupt) {
		t.Fatal("a broken stored time must be reported as damage", err)
	}
}

// plan adds items to the test day, in order, a second apart from from, and
// returns their IDs.
func plan(t *testing.T, s *Store, day domain.Day, from string, titles ...string) []domain.ID {
	t.Helper()
	var ids []domain.ID
	for i, title := range titles {
		it, _ := domain.NewItem(day.ID, title, 30, clock(from).Add(time.Duration(i)*time.Second))
		if err := s.Create(ctx, nil, Change{Item: it, Event: domain.Record(nil, it, it.CreatedAt, it.CreatedAt, domain.ReasonUser)}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, it.ID)
	}
	return ids
}

func order(t *testing.T, s *Store, day domain.Day) string {
	t.Helper()
	items, err := s.Items(ctx, day.ID)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, it := range items {
		titles = append(titles, it.Title)
	}
	return strings.Join(titles, " ")
}

// New items go to the end, and a saved order is kept exactly.
func TestTheOrderIsWhatWasSaved(t *testing.T) {
	s := openTest(t)
	day, first := createTestItem(t, s)
	ids := plan(t, s, day, "08:10:00", "B", "C", "D")
	if got := order(t, s, day); got != "Study B C D" {
		t.Fatalf("got %s", got)
	}
	if err := s.SetOrder(ctx, day.ID, []domain.ID{first.ID, ids[2], ids[0], ids[1]}); err != nil {
		t.Fatal(err)
	}
	if got := order(t, s, day); got != "Study D B C" {
		t.Fatalf("got %s", got)
	}
	plan(t, s, day, "08:20:00", "E")
	if got := order(t, s, day); got != "Study D B C E" {
		t.Fatalf("a new item goes to the end, got %s", got)
	}
}

// An order that leaves an item out, names one twice or names a stranger is
// refused, and the order stays as it was.
func TestAnOrderMustNameEveryItemOnce(t *testing.T) {
	s := openTest(t)
	day, first := createTestItem(t, s)
	ids := plan(t, s, day, "08:10:00", "B", "C")
	for name, bad := range map[string][]domain.ID{
		"missing one": {first.ID, ids[0]},
		"one twice":   {first.ID, ids[0], ids[0]},
		"a stranger":  {first.ID, ids[0], "it_stranger"},
	} {
		if err := s.SetOrder(ctx, day.ID, bad); !errors.Is(err, ErrOrderChanged) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if got := order(t, s, day); got != "Study B C" {
		t.Fatalf("the order changed: %s", got)
	}
}

// A deleted item gives up its place, and the rest close the gap.
func TestDeletingClosesTheGap(t *testing.T) {
	s := openTest(t)
	day, first := createTestItem(t, s)
	ids := plan(t, s, day, "08:10:00", "B", "C")
	b, err := s.Item(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	before := b.Clone()
	_ = b.Archive(clock("09:00:00"))
	if err := s.Save(ctx, Change{b, domain.Record(&before, b, clock("09:00:00"), clock("09:00:00"), domain.ReasonUser)}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOrder(ctx, day.ID, []domain.ID{ids[1], first.ID}); err != nil {
		t.Fatalf("the two left should be numbered without a gap: %v", err)
	}
	if got := order(t, s, day); got != "C Study" {
		t.Fatalf("got %s", got)
	}
}
