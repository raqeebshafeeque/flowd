package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/utils"
	"golang.org/x/sys/unix"
	"modernc.org/sqlite"
)

func TestSQLiteFullPreservesCommittedData(t *testing.T) {
	store := blockTestStore(t)
	var pages int
	if err := store.db.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(fmt.Sprintf("PRAGMA max_page_count = %d", pages)); err != nil {
		t.Fatal(err)
	}
	for number := 0; number < 2000; number++ {
		day := exampleDay(t)
		day.ID = utils.ID(fmt.Sprintf("dy_fill_%d", number))
		day.Date = day.CreatedAt.AddDate(0, 0, number+1).Format("2006-01-02")
		err := store.CreateDay(context.Background(), day)
		if err == nil {
			continue
		}
		var driver *sqlite.Error
		if !errors.Is(err, ErrDiskFull) || !errors.As(err, &driver) {
			t.Fatalf("want full category and driver cause, got %v", err)
		}
		got, err := store.DayByID(context.Background(), "dy_test")
		if err != nil {
			t.Fatal(err)
		}
		assertDay(t, got, exampleDay(t))
		return
	}
	t.Fatal("expected the bounded SQLite page limit to be reached")
}

func TestFilesystemFullClassification(t *testing.T) {
	original := &os.PathError{Op: "write", Path: "backup", Err: unix.ENOSPC}
	err := classify(original)
	var pathError *os.PathError
	if !errors.Is(err, ErrDiskFull) || !errors.Is(err, unix.ENOSPC) || !errors.As(err, &pathError) {
		t.Errorf("full filesystem cause should be retained, got %v", err)
	}
}
