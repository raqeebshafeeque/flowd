package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// session runs commands against one database with a clock the test moves.
type session struct {
	t   *testing.T
	db  string
	now time.Time
}

func newSession(t *testing.T) *session {
	return &session{t: t, db: filepath.Join(t.TempDir(), "flowd.sqlite"), now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
}

func (s *session) run(args ...string) (stdout, stderr string, code int) {
	var out, errOut bytes.Buffer
	env := Env{Version: "1.2.3", Now: func() time.Time { return s.now }, Zone: time.UTC}
	code = Run(context.Background(), append([]string{"--db", s.db}, args...), &out, &errOut, env)
	return out.String(), errOut.String(), code
}

func (s *session) ok(args ...string) string {
	s.t.Helper()
	out, errOut, code := s.run(args...)
	if code != 0 {
		s.t.Fatalf("%v exited %d: %s", args, code, errOut)
	}
	return out
}

func (s *session) fails(code int, errCode string, args ...string) {
	s.t.Helper()
	out, errOut, got := s.run(args...)
	if got != code || out != "" || !strings.HasPrefix(errOut, errCode+"\n") {
		s.t.Fatalf("%v: exit %d, stdout %q, stderr %q, want %d %s", args, got, out, errOut, code, errCode)
	}
}

func contains(text, part string) bool { return strings.Contains(text, part) }

func TestADayFromStartToReview(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Study")
	s.ok("add", "30m", "Code", "review")
	if out := s.ok("start", "1"); !contains(out, "Started Study") {
		t.Fatalf("got %q", out)
	}
	s.now = s.now.Add(40 * time.Minute)
	if out := s.ok("start", "2"); !contains(out, "Started Code review") || !contains(out, "Paused Study") {
		t.Fatalf("got %q", out)
	}
	s.now = s.now.Add(20 * time.Minute)
	s.ok("finish", "2")
	list := s.ok("list")
	for _, want := range []string{"Tue 17 Nov 2026 · UTC", "PAUSED", "40m / 1h", "Study", "DONE", "20m / 30m"} {
		if !contains(list, want) {
			t.Errorf("list is missing %q:\n%s", want, list)
		}
	}
	review := s.ok("review")
	for _, want := range []string{"Total duration   1h30m", "Recorded work    1h", "1 / 2 finished", "DONE (1)", "• Code review", "INCOMPLETE (0)"} {
		if !contains(review, want) {
			t.Errorf("review is missing %q:\n%s", want, review)
		}
	}
}

func TestJSONOutput(t *testing.T) {
	s := newSession(t)
	var created map[string]any
	if err := json.Unmarshal([]byte(s.ok("--json", "add", "1h", "Study")), &created); err != nil {
		t.Fatal(err)
	}
	if created["title"] != "Study" || created["duration_minutes"] != float64(60) || created["elapsed_ns"] != nil {
		t.Fatalf("unexpected item: %v", created)
	}
	if strings.TrimSpace(s.ok("--json", "now")) != "null" {
		t.Fatal("nothing running must be null")
	}
	s.ok("start", created["id"].(string))
	var day map[string]any
	if err := json.Unmarshal([]byte(s.ok("--json")), &day); err != nil {
		t.Fatal(err)
	}
	if day["schema_version"] != float64(1) || day["running"] == nil || len(day["items"].([]any)) != 1 {
		t.Fatalf("unexpected day: %v", day)
	}
	var events []map[string]any
	if err := json.Unmarshal([]byte(s.ok("--json", "events", "1")), &events); err != nil || len(events) != 2 {
		t.Fatalf("events: %v %v", events, err)
	}
}

func TestErrorsHaveCodesAndExitStatuses(t *testing.T) {
	s := newSession(t)
	s.fails(2, "ERR_USAGE", "dance")
	s.fails(2, "ERR_DURATION", "add", "1.5h", "Study")
	s.fails(1, "ERR_NOT_FOUND", "start", "1")
	s.ok("add", "1h", "Study")
	s.fails(1, "ERR_ILLEGAL_TRANSITION", "pause", "1")
	s.fails(2, "ERR_PAST_DAY", "--date", "2026-11-16", "add", "1h", "Late")
	s.ok("--date", "2026-11-18", "add", "1h", "Tomorrow")
	s.fails(1, "ERR_DAY_CLOSED", "--date", "2026-11-18", "start", "1")
	s.fails(2, "ERR_USAGE", "--json", "add", "1h")
}

func TestEditDeleteAndEveryAction(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Study")
	if out := s.ok("edit", "--title", "Deep study", "--duration", "2h", "1"); !contains(out, "Updated Deep study (TODO, 2h)") {
		t.Fatalf("got %q", out)
	}
	for _, step := range []string{"start", "pause", "resume", "finish", "reopen", "skip", "unskip"} {
		s.now = s.now.Add(time.Minute)
		s.ok(step, "1")
	}
	s.ok("delete", "1")
	if out := s.ok("list"); !contains(out, "Nothing planned") {
		t.Fatalf("a deleted item is still listed: %q", out)
	}
}

func TestMaintenanceCommands(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Study")
	if out := s.ok("backup"); !contains(out, s.db+".bak") {
		t.Fatalf("got %q", out)
	}
	if out := s.ok("doctor"); out != "OK\n" {
		t.Fatalf("got %q", out)
	}
	if err := os.WriteFile(s.db, []byte(strings.Repeat("broken ", 2000)), 0o600); err != nil {
		t.Fatal(err)
	}
	s.fails(1, "ERR_CORRUPT", "doctor")
	s.fails(1, "ERR_CORRUPT", "list")
	if out := s.ok("doctor", "--repair"); !contains(out, "Restored the backup") {
		t.Fatalf("got %q", out)
	}
	if out := s.ok("list"); !contains(out, "Study") {
		t.Fatal("the restored item is missing")
	}
}

func TestHelpAndVersionTouchNoFiles(t *testing.T) {
	s := newSession(t)
	if out := s.ok("--version"); out != "flowd 1.2.3\n" {
		t.Fatalf("got %q", out)
	}
	if out := s.ok("--help"); !contains(out, "Usage:") {
		t.Fatal("no help")
	}
	if _, err := os.Stat(s.db); !os.IsNotExist(err) {
		t.Fatal("help or version created the database")
	}
}

func TestHiddenCharactersInTitlesAreRefused(t *testing.T) {
	s := newSession(t)
	_, errOut, code := s.run("add", "1h", "Pay invoice\u202e fdp.exe")
	if code != 2 || !strings.HasPrefix(errOut, "ERR_TITLE\n") || !contains(errOut, "Titles cannot hold hidden or direction-changing characters") || !contains(errOut, "Remove the invisible or direction-changing character U+202E at position 12") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	s.ok("add", "1h", "Ship it 👩\u200d💻 ✔︎")
	s.fails(2, "ERR_TITLE", "edit", "--title", "zero\u200bwidth", "1")
	if out := s.ok("doctor"); out != "OK\n" {
		t.Fatalf("a clean database should not warn: %q", out)
	}
}
