package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/raqeebshafeeque/flowd/internal/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "flowd.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenCreatesAPrivateDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "flowd.sqlite")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("database mode %v, %v", info.Mode().Perm(), err)
	}
	dir, _ := os.Stat(filepath.Dir(path))
	if dir.Mode().Perm() != 0o700 {
		t.Fatalf("new folder mode %v", dir.Mode().Perm())
	}
	again, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal("reopening failed:", err)
	}
	again.Close()
}

func TestSecondOpenIsRefusedWhileTheFirstIsOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flowd.sqlite")
	first, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), path); !errors.Is(err, ErrLocked) {
		t.Fatal("a second owner must be refused", err)
	}
	first.Close()
	second, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal("the lock was not released:", err)
	}
	second.Close()
}

func TestDamagedFilesAreRefusedAndKept(t *testing.T) {
	cases := map[string][]byte{
		"empty file":     {},
		"not a database": []byte(strings.Repeat("this is not sqlite ", 300)),
	}
	for name, content := range cases {
		path := filepath.Join(t.TempDir(), "flowd.sqlite")
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(context.Background(), path); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: got %v", name, err)
		}
		if after, _ := os.ReadFile(path); string(after) != string(content) {
			t.Errorf("%s: the file was changed", name)
		}
	}
}

func TestUnsafePathsAreRefused(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.sqlite")
	s, err := Open(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	hard := filepath.Join(dir, "hard.sqlite")
	if err := os.Link(target, hard); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), hard); !errors.Is(err, ErrInvalidPath) {
		t.Error("a hard-linked database must be refused", err)
	}
	if _, err := Open(context.Background(), dir); !errors.Is(err, ErrInvalidPath) {
		t.Error("a directory must be refused", err)
	}
	if _, err := Open(context.Background(), ""); !errors.Is(err, ErrInvalidPath) {
		t.Error("an empty path must be refused", err)
	}
}

func TestSharedFilesAreRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flowd.sqlite")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), path); !errors.Is(err, ErrPermissions) {
		t.Fatal("a database others can read must be refused", err)
	}
}

func TestSymlinkToADatabaseIsFollowed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.sqlite")
	created, err := Open(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	created.Close()
	link := filepath.Join(dir, "link.sqlite")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if want, _ := filepath.EvalSymlinks(target); s.Path() != want {
		t.Fatalf("got path %q, want the link target %q", s.Path(), want)
	}
	dangling := filepath.Join(dir, "dangling.sqlite")
	if err := os.Symlink(filepath.Join(dir, "missing.sqlite"), dangling); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), dangling); !errors.Is(err, ErrInvalidPath) {
		t.Fatal("a link to nothing must be refused, not followed into a new file", err)
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data")
	if got, _ := DefaultPath(); got != "/data/flowd/flowd.sqlite" {
		t.Errorf("got %q", got)
	}
	t.Setenv("XDG_DATA_HOME", "relative")
	t.Setenv("HOME", "/home/me")
	if got, _ := DefaultPath(); got != "/home/me/.local/share/flowd/flowd.sqlite" {
		t.Errorf("a relative XDG_DATA_HOME must be ignored, got %q", got)
	}
}

func chmodPrivate(path string) error { return os.Chmod(path, 0o600) }

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
		if _, err := Open(ctx, path); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: got %v", name, err)
		}
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
		if _, err := tx.Exec("INSERT INTO items("+itemColumns+",position) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", append(itemValues(item), carriedFrom(item), nil, i+1)...); err != nil {
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

// Every open checks the whole file for damage. The budget is far above the
// measured time, so it only fails when opening starts doing much more.
func TestOpeningALargeHistoryIsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("timing budgets only run without -short, and never under -race")
	}
	path := largeHistory(t, 2000)
	var slowest time.Duration
	for i := 0; i < 5; i++ {
		start := time.Now()
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		s.Close()
		slowest = max(slowest, time.Since(start))
	}
	if slowest > 150*time.Millisecond {
		t.Errorf("opening 2000 items took %v, budget 150ms", slowest)
	}
}

// Older versions left a quick check record next to the file. Opening clears
// it away.
func TestTheOldCheckRecordIsClearedAway(t *testing.T) {
	path := savedDatabase(t)
	if err := os.WriteFile(path+".check", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := os.Stat(path + ".check"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the old record is still there", err)
	}
}
