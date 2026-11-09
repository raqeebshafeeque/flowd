package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"

	"modernc.org/sqlite"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/utils"
)

func TestCreateDay(t *testing.T) {
	updated := testTimestamp(t, "2026-10-14T12:00:00.987654321+05:30")
	for _, test := range []struct {
		name    string
		updated *time.Time
	}{
		{"no update time", nil}, {"populated update time", &updated},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openTestStore(t)
			day := exampleDay(t)
			day.UpdatedAt = test.updated
			if err := store.CreateDay(context.Background(), day); err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			var id, date, zone, created string
			var savedUpdate sql.NullString
			err := store.db.QueryRow(`SELECT id, date, timezone, created_at_utc, updated_at_utc
				FROM days WHERE id = ?`, day.ID).Scan(&id, &date, &zone, &created, &savedUpdate)
			if err != nil {
				t.Fatal(err)
			}
			if id != string(day.ID) || date != day.Date || zone != day.Timezone {
				t.Error("saved identity, date and timezone should match the input")
			}
			if created != day.CreatedAt.UTC().Format(time.RFC3339Nano) {
				t.Errorf("got creation time %q, want UTC with preserved precision", created)
			}
			if test.updated == nil {
				if savedUpdate.Valid {
					t.Error("absent update time should be SQL NULL")
				}
			} else if !savedUpdate.Valid || savedUpdate.String != updated.UTC().Format(time.RFC3339Nano) {
				t.Error("update time should retain its instant in UTC")
			}
		})
	}
}

func TestDayLookup(t *testing.T) {
	store := openTestStore(t)
	day := exampleDay(t)
	updated := day.CreatedAt.Add(time.Hour)
	day.UpdatedAt = &updated
	if err := store.CreateDay(context.Background(), day); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, value string
		find        func(string) (*domain.Day, error)
	}{
		{"by date", day.Date, func(value string) (*domain.Day, error) { return store.DayByDate(context.Background(), value) }},
		{"by id", string(day.ID), func(value string) (*domain.Day, error) { return store.DayByID(context.Background(), utils.ID(value)) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.find(test.value)
			if err != nil {
				t.Fatal(err)
			}
			assertDay(t, got, day)
			if got.CreatedAt.Location() != time.UTC {
				t.Error("creation time should be UTC")
			}
			if _, err := test.find("missing"); !errors.Is(err, ErrDayNotFound) {
				t.Errorf("got error %v, want %v", err, ErrDayNotFound)
			}
		})
	}
}

func TestDayFailures(t *testing.T) {
	for _, kind := range []string{"duplicate id", "duplicate date", "creation time", "update time"} {
		t.Run(kind, func(t *testing.T) {
			store := openTestStore(t)
			day := exampleDay(t)
			if err := store.CreateDay(context.Background(), day); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(kind, "duplicate") {
				other := day
				if kind == "duplicate id" {
					other.Date = "2026-10-15"
				} else {
					other.ID = "dy_other"
				}
				var cause *sqlite.Error
				if err := store.CreateDay(context.Background(), other); !errors.As(err, &cause) {
					t.Fatalf("want SQLite constraint cause, got %v", err)
				}
				got, err := store.DayByID(context.Background(), day.ID)
				if err != nil {
					t.Fatal(err)
				}
				assertDay(t, got, day)
				return
			}
			column := "created_at_utc"
			if kind == "update time" {
				column = "updated_at_utc"
			}
			if _, err := store.db.Exec("UPDATE days SET " + column + " = 'invalid'"); err != nil {
				t.Fatal(err)
			}
			_, err := store.DayByDate(context.Background(), day.Date)
			var cause *time.ParseError
			if !errors.As(err, &cause) || !strings.Contains(err.Error(), kind) {
				t.Errorf("want %s context and parsing cause, got %v", kind, err)
			}
		})
	}
}

func TestDayCancellationAndLiteralValues(t *testing.T) {
	store := openTestStore(t)
	day := exampleDay(t)
	day.ID = "dy_'; DROP TABLE days; --"
	day.Timezone = "UTC'; DROP TABLE blocks; --"
	if err := store.CreateDay(context.Background(), day); err != nil {
		t.Fatal(err)
	}
	got, err := store.DayByID(context.Background(), day.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertDay(t, got, day)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.CreateDay(ctx, day); !errors.Is(err, context.Canceled) {
		t.Errorf("want cancellation, got %v", err)
	}
	if _, err := store.DayByDate(ctx, day.Date); !errors.Is(err, context.Canceled) {
		t.Errorf("want cancellation, got %v", err)
	}
	if _, err := store.DayByID(ctx, day.ID); !errors.Is(err, context.Canceled) {
		t.Errorf("want cancellation, got %v", err)
	}
}

func TestDayReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flowd.sqlite")
	first, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	day := exampleDay(t)
	if err := first.CreateDay(context.Background(), day); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	got, err := second.DayByDate(context.Background(), day.Date)
	if err != nil {
		t.Fatal(err)
	}
	assertDay(t, got, day)
}

func assertDay(t *testing.T, got *domain.Day, want domain.Day) {
	t.Helper()
	if got == nil {
		t.Fatal("want a saved day, got nil")
	}
	if got.ID != want.ID {
		t.Errorf("got id %q, want %q", got.ID, want.ID)
	}
	if got.Date != want.Date {
		t.Errorf("got date %q, want %q", got.Date, want.Date)
	}
	if got.Timezone != want.Timezone {
		t.Errorf("got timezone %q, want %q", got.Timezone, want.Timezone)
	}
	assertTime(t, "creation time", &got.CreatedAt, &want.CreatedAt)
	assertTime(t, "update time", got.UpdatedAt, want.UpdatedAt)
}

func assertTime(t *testing.T, name string, got, want *time.Time) {
	t.Helper()
	if got == nil || want == nil {
		if (got == nil) != (want == nil) {
			t.Errorf("missing value differs for %s", name)
		}
	} else if !got.Equal(*want) {
		t.Errorf("got %s %v, want %v", name, got, want)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "flowd.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store
}

func testTimestamp(t *testing.T, value string) time.Time {
	t.Helper()
	stamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return stamp
}

func exampleDay(t *testing.T) domain.Day {
	t.Helper()
	return domain.Day{ID: "dy_test", Date: "2026-10-14", Timezone: "Asia/Kolkata",
		CreatedAt: testTimestamp(t, "2026-10-14T09:00:00.123456789+05:30")}
}
