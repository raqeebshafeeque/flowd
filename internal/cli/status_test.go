package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTheStatusLineShowsTheRunningItem(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Deep work on the parser")
	s.ok("add", "30m", "Inbox")
	s.ok("start", s.id("Deep work on the parser"))
	s.now = s.now.Add(42*time.Minute + 50*time.Second)
	if out := s.ok("now", "--short"); out != "Deep work on the parser 42m/1h\n" {
		t.Fatalf("got %q", out)
	}
	s.now = s.now.Add(30 * time.Minute)
	if out := s.ok("now", "--short"); out != "Deep work on the parser 1h12m/1h +12m\n" {
		t.Fatalf("past its duration: %q", out)
	}
	s.ok("pause", s.id("Deep work on the parser"))
	if out := s.ok("now", "--short"); out != "" {
		t.Fatalf("nothing running should print nothing: %q", out)
	}
}

// A prompt is the wrong place for an error. A missing or broken database
// prints nothing and still succeeds, and nothing is created.
func TestTheStatusLineNeverFailsLoudly(t *testing.T) {
	s := newSession(t)
	if out, errOut, code := s.run("now", "--short"); out != "" || errOut != "" || code != 0 {
		t.Fatalf("missing: %q %q %d", out, errOut, code)
	}
	if _, err := os.Stat(s.db); err == nil {
		t.Fatal("the status line must not create the database")
	}
	os.WriteFile(s.db, []byte("not a database at all, just some text"), 0o600)
	if out, errOut, code := s.run("now", "--short"); out != "" || errOut != "" || code != 0 {
		t.Fatalf("broken: %q %q %d", out, errOut, code)
	}
}

// After midnight the running item belongs to yesterday. The status line
// shows nothing, and leaves closing the day to the next real command.
func TestTheStatusLineDoesNotCloseADay(t *testing.T) {
	s := newSession(t)
	s.ok("add", "30m", "Late")
	s.ok("start", s.id("Late"))
	// The database and anything saved in its WAL must stay exactly as they
	// were. SQLite may leave an empty WAL next to it, which holds nothing.
	files := func() string {
		var sizes []string
		for _, suffix := range []string{"", "-wal"} {
			if info, err := os.Stat(s.db + suffix); err == nil && info.Size() > 0 {
				sizes = append(sizes, fmt.Sprint(suffix, " ", info.ModTime().UnixNano(), " ", info.Size()))
			}
		}
		return strings.Join(sizes, ", ")
	}
	before := files()
	s.now = time.Date(2026, 11, 18, 0, 30, 0, 0, time.UTC)
	if out := s.ok("now", "--short"); out != "" {
		t.Fatalf("got %q", out)
	}
	if files() != before {
		t.Fatalf("the status line wrote to the database:\n%s\n%s", before, files())
	}
	if day := s.ok("--json", "--date", "2026-11-17"); !strings.Contains(day, `"status":"incomplete"`) {
		t.Fatalf("the next real command should still close the day: %s", day)
	}
}

// The status line reads so often that it must stay cheap. The budget is far
// above what it takes, so it only fails if it starts doing much more.
func TestTheStatusLineIsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("timing budgets only run without -short")
	}
	s := newSession(t)
	for i := 0; i < 20; i++ {
		s.ok("add", "10m", "Item")
	}
	s.ok("start", s.id("Item"))
	start := time.Now()
	for i := 0; i < 50; i++ {
		s.ok("now", "--short")
	}
	if per := time.Since(start) / 50; per > 25*time.Millisecond {
		t.Errorf("the status line took %v, budget 25ms", per)
	}
}
