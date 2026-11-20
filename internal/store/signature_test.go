package store

import (
	"os"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

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
	act(t, s, day, items[0], domain.Start, clock("09:00:00"))
	after, _ := StatSignature(path)
	s.Close()
	if after == before {
		t.Fatal("a committed change must change the signature")
	}
}

// A status line reads without the lock and can leave an empty WAL behind.
// That holds nothing, so it must not count as a change.
func TestAnEmptyWALIsNotAChange(t *testing.T) {
	path := savedDatabase(t)
	signature, _ := StatSignature(path)
	if err := os.WriteFile(path+"-wal", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if again, _ := StatSignature(path); again != signature {
		t.Fatal("an empty WAL changed the signature")
	}
	if err := os.WriteFile(path+"-wal", []byte("frames"), 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, _ := StatSignature(path); changed == signature {
		t.Fatal("a WAL with content must count as a change")
	}
}
