package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// opens counts the full scans one open runs.
func opens(t *testing.T, path string, options ...OpenOption) (int64, error) {
	t.Helper()
	before := scans.Load()
	s, err := Open(ctx, path, options...)
	if err != nil {
		return scans.Load() - before, err
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return scans.Load() - before, nil
}

func savedDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flowd.sqlite")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	createTestItem(t, s)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnUnchangedDatabaseIsNotScannedAgain(t *testing.T) {
	path := savedDatabase(t)
	for i := 0; i < 3; i++ {
		if n, err := opens(t, path); err != nil || n != 0 {
			t.Fatalf("open %d: %d scans, %v", i, n, err)
		}
	}
}

func TestFlowdsOwnWritesDoNotCauseAScan(t *testing.T) {
	path := savedDatabase(t)
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	day, _ := s.DayByDate(ctx, "2026-11-17")
	items, _ := s.Items(ctx, day.ID)
	act(t, s, day, items[0], domain.Start, clock("09:00:00"))
	s.Close()
	if n, err := opens(t, path); err != nil || n != 0 {
		t.Fatalf("%d scans, %v", n, err)
	}
}

func TestAChangeFromOutsideCausesAScan(t *testing.T) {
	path := savedDatabase(t)
	db, err := connect(ctx, path, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE days SET updated_at=created_at"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if n, err := opens(t, path); err != nil || n != 1 {
		t.Fatalf("%d scans, %v", n, err)
	}
	if n, _ := opens(t, path); n != 0 {
		t.Fatal("after one scan and a clean close the file is trusted again")
	}
}

func TestDamageIsStillFound(t *testing.T) {
	cases := map[string]func(string){
		"truncated": func(p string) { os.Truncate(p, 3000) },
		"broken page header": func(p string) {
			f, _ := os.OpenFile(p, os.O_RDWR, 0)
			f.WriteAt([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, 4096)
			f.Close()
		},
	}
	for name, damage := range cases {
		path := savedDatabase(t)
		damage(path)
		if _, err := opens(t, path); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestLeftoversAndBadRecordsCauseAScan(t *testing.T) {
	cases := map[string]func(string){
		"missing record": func(p string) { os.Remove(recordPath(p)) },
		"corrupt record": func(p string) { os.WriteFile(recordPath(p), []byte("{nope"), 0o600) },
		"old record": func(p string) {
			r, _ := readRecord(p)
			r.ScannedAt = time.Now().Add(-25 * time.Hour)
			writeRecord(p, r)
		},
		"future record": func(p string) { r, _ := readRecord(p); r.ScannedAt = time.Now().Add(time.Hour); writeRecord(p, r) },
		"journal left":  func(p string) { os.WriteFile(p+"-journal", nil, 0o600) },
		"other version": func(p string) { r, _ := readRecord(p); r.SchemaVersion = 99; writeRecord(p, r) },
	}
	for name, change := range cases {
		path := savedDatabase(t)
		change(path)
		n, err := opens(t, path)
		if name == "journal left" && err != nil {
			continue // SQLite may refuse a stray journal outright, which is also safe
		}
		if err != nil || n != 1 {
			t.Errorf("%s: %d scans, %v", name, n, err)
		}
	}
}

func TestAFullCheckCanBeAskedFor(t *testing.T) {
	path := savedDatabase(t)
	if n, err := opens(t, path, WithFullCheck()); err != nil || n != 1 {
		t.Fatalf("%d scans, %v", n, err)
	}
}

func TestTheSchemaIsCheckedEvenWithoutAScan(t *testing.T) {
	path := savedDatabase(t)
	db, _ := connect(ctx, path, false)
	db.Exec("PRAGMA user_version = 7")
	db.Close()
	s, _ := Open(ctx, filepath.Join(t.TempDir(), "x.sqlite"))
	s.Close()
	r, _ := readRecord(path)
	f, _, _ := fingerprintOf(path)
	r.Fingerprint = f // pretend the record matches the changed file
	writeRecord(path, r)
	if n, err := opens(t, path); !errors.Is(err, ErrVersion) || n != 0 {
		t.Fatalf("a matching record must never skip the version check: %d scans, %v", n, err)
	}
}

func TestTheRecordIsPrivate(t *testing.T) {
	path := savedDatabase(t)
	info, err := os.Stat(recordPath(path))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("record mode %v, %v", info.Mode().Perm(), err)
	}
}

func TestTheSignatureChangesWhenTheFileIsWritten(t *testing.T) {
	path := savedDatabase(t)
	before, err := StatSignature(path)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := StatSignature(path); again != before {
		t.Fatal("an unchanged file must keep its signature")
	}
	time.Sleep(10 * time.Millisecond)
	s, _ := Open(ctx, path)
	day, _ := s.DayByDate(ctx, "2026-11-17")
	items, _ := s.Items(ctx, day.ID)
	act(t, s, day, items[0], domain.Skip, clock("09:00:00"))
	after, _ := StatSignature(path)
	s.Close()
	if after == before {
		t.Fatal("a committed change must change the signature")
	}
}

// largeHistory builds a database with n finished items, three events each,
// in one transaction: about two years of daily use for n = 2000.
func largeHistory(tb testing.TB, n int) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "flowd.sqlite")
	s, err := Open(ctx, path)
	if err != nil {
		tb.Fatal(err)
	}
	day := domain.Day{ID: "dy_big", Date: "2026-11-17", Timezone: "UTC", CreatedAt: clock("00:00:00")}
	tx, err := s.db.Begin()
	if err != nil {
		tb.Fatal(err)
	}
	tx.Exec("INSERT INTO days("+dayColumns+") VALUES(?,?,?,?,?)", day.ID, day.Date, day.Timezone, instant(day.CreatedAt), nil)
	for i := 0; i < n; i++ {
		item := domain.Item{ID: domain.ID(fmt.Sprintf("it_%05d", i)), DayID: day.ID, Title: "Representative item", Status: domain.Todo, DurationMinutes: 30, CreatedAt: clock("01:00:00")}
		events := []domain.Event{domain.Record(nil, item, item.CreatedAt, item.CreatedAt, domain.ReasonUser)}
		for j, a := range []domain.Action{domain.Start, domain.Finish} {
			before := item.Clone()
			at := clock("01:00:00").Add(time.Duration(j+1) * time.Minute)
			if err := item.Apply(a, at, day); err != nil {
				tb.Fatal(err)
			}
			events = append(events, domain.Record(&before, item, at, at, domain.ReasonUser))
		}
		if _, err := tx.Exec("INSERT INTO items("+itemColumns+",position) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", append(itemValues(item), carriedFrom(item), i+1)...); err != nil {
			tb.Fatal(err)
		}
		for _, e := range events {
			before, _ := encode(e.Before)
			after, _ := encode(&e.After)
			if _, err := tx.Exec("INSERT INTO item_events(item_id,from_status,to_status,occurred_at,recorded_at,reason,before_json,after_json) VALUES(?,?,?,?,?,?,?,?)", e.ItemID, e.FromStatus, e.ToStatus, instant(e.OccurredAt), instant(e.RecordedAt), e.Reason, before, after); err != nil {
				tb.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		tb.Fatal(err)
	}
	if err := s.Close(); err != nil {
		tb.Fatal(err)
	}
	return path
}

// The budget is far above the measured time, so it only fails when opening
// goes back to scanning the whole file, not on a slow machine.
func TestOpeningALargeHistoryIsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("timing budgets only run without -short, and never under -race")
	}
	path := largeHistory(t, 2000)
	var slowest time.Duration
	for i := 0; i < 5; i++ {
		start := time.Now()
		n, err := opens(t, path)
		if err != nil || n != 0 {
			t.Fatalf("%d scans, %v", n, err)
		}
		slowest = max(slowest, time.Since(start))
	}
	if slowest > 100*time.Millisecond {
		t.Errorf("opening 2000 items took %v, budget 100ms", slowest)
	}
}

func BenchmarkOpenLargeHistory(b *testing.B) {
	path := largeHistory(b, 2000)
	for i := 0; i < b.N; i++ {
		s, err := Open(ctx, path)
		if err != nil {
			b.Fatal(err)
		}
		s.Close()
	}
}

func BenchmarkOpenLargeHistoryWithFullCheck(b *testing.B) {
	path := largeHistory(b, 2000)
	for i := 0; i < b.N; i++ {
		s, err := Open(ctx, path, WithFullCheck())
		if err != nil {
			b.Fatal(err)
		}
		s.Close()
	}
}

// A status line reads without the lock and can leave an empty WAL behind.
// That holds nothing, so it must not count as a change and force a scan.
func TestAnEmptyWALIsNotAChange(t *testing.T) {
	s := openTest(t)
	createTestItem(t, s)
	path := s.Path()
	s.Close()
	before, _, err := fingerprintOf(path)
	if err != nil {
		t.Fatal(err)
	}
	signature, _ := StatSignature(path)
	if err := os.WriteFile(path+"-wal", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	after, _, _ := fingerprintOf(path)
	if after != before {
		t.Fatalf("an empty WAL changed the fingerprint: %+v to %+v", before, after)
	}
	if again, _ := StatSignature(path); again != signature {
		t.Fatal("an empty WAL changed the signature")
	}
	if err := os.WriteFile(path+"-wal", []byte("frames"), 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, _, _ := fingerprintOf(path); changed == before {
		t.Fatal("a WAL with content must count as a change")
	}
}
