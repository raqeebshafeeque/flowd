package cli

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// session runs commands against one database with a clock the test moves.
type session struct {
	t   *testing.T
	db  string
	now time.Time
	// answer, when set, is typed at a terminal for any question.
	answer *string
	// keys, when set, are pressed in the item picker, as at a terminal.
	keys []string
	// offered is what the picker showed, for checking.
	offered []string
	// picker, when set, runs the picker instead of pressing keys.
	picker func(tea.Model) (tea.Model, error)
}

func newSession(t *testing.T) *session {
	return &session{t: t, db: filepath.Join(t.TempDir(), "flowd.sqlite"), now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
}

func (s *session) run(args ...string) (stdout, stderr string, code int) {
	var out, errOut bytes.Buffer
	env := Env{Version: "1.2.3", Now: func() time.Time { return s.now }, Zone: time.UTC}
	if s.answer != nil {
		env.In, env.Interactive = strings.NewReader(*s.answer), true
	}
	if s.keys != nil {
		env.Interactive = true
		env.pick = s.press
		if s.picker != nil {
			env.pick = s.picker
		}
	}
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

// fails checks a refused command both ways: as JSON with its code, the way
// a script sees it, and as plain words, the way a person does.
func (s *session) fails(code int, errCode string, args ...string) {
	s.t.Helper()
	out, errOut, got := s.run(append([]string{"--json"}, args...)...)
	if got != code || out != "" || jsonCode(s.t, errOut) != errCode {
		s.t.Fatalf("%v: exit %d, stdout %q, stderr %q, want %d %s", args, got, out, errOut, code, errCode)
	}
	if !slices.Contains(args, "--json") {
		out, errOut, got = s.run(args...)
		if got != code || out != "" || !strings.HasPrefix(errOut, "Error: ") || contains(errOut, "ERR_") {
			s.t.Fatalf("%v: exit %d, stdout %q, stderr %q", args, got, out, errOut)
		}
	}
}

func jsonCode(t *testing.T, errOut string) string {
	t.Helper()
	var e struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if err := json.Unmarshal([]byte(errOut), &e); err != nil || e.Error.Message == "" {
		t.Fatalf("not a JSON error: %q", errOut)
	}
	return e.Error.Code
}

func contains(text, part string) bool { return strings.Contains(text, part) }

// id returns the full ID of the item with this title, read as JSON the way
// a script would. Extra arguments such as --date pick the day.
func (s *session) id(title string, args ...string) string {
	s.t.Helper()
	var day struct {
		Items []struct{ ID, Title string } `json:"items"`
	}
	if err := json.Unmarshal([]byte(s.ok(append(append([]string{"--json"}, args...), "list")...)), &day); err != nil {
		s.t.Fatal(err)
	}
	for _, item := range day.Items {
		if item.Title == title {
			return item.ID
		}
	}
	s.t.Fatalf("no item %q", title)
	return ""
}

func TestADayFromStartToReview(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Study")
	s.ok("add", "30m", "Code", "review")
	if out := s.ok("start", s.id("Study")); !contains(out, "│ Study │") || !contains(out, "IN PROGRESS") {
		t.Fatalf("got %q", out)
	}
	s.now = s.now.Add(40 * time.Minute)
	// Only the item acted on is shown, not the one it paused.
	if out := s.ok("start", s.id("Code review")); !contains(out, "Code review") || !contains(out, "IN PROGRESS") || contains(out, "Study") {
		t.Fatalf("got %q", out)
	}
	s.now = s.now.Add(20 * time.Minute)
	s.ok("finish", s.id("Code review"))
	list := s.ok("list")
	if !strings.HasPrefix(list, "Tue 17 Nov 2026\n┌") {
		t.Errorf("the date should sit right above the table:\n%s", list)
	}
	for _, want := range []string{"Tue 17 Nov 2026", "PAUSED", "40m / 1h", "Study", "DONE", "20m / 30m"} {
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
	if created["title"] != "Study" || created["duration_minutes"] != float64(60) || created["paused_ns"] != nil {
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
	if day["schema_version"] != float64(2) || day["running"] == nil || len(day["items"].([]any)) != 1 {
		t.Fatalf("unexpected day: %v", day)
	}
	var events []map[string]any
	if err := json.Unmarshal([]byte(s.ok("--json", "events", s.id("Study"))), &events); err != nil || len(events) != 2 {
		t.Fatalf("events: %v %v", events, err)
	}
}

func TestErrorsHaveCodesAndExitStatuses(t *testing.T) {
	s := newSession(t)
	s.fails(2, "ERR_USAGE", "dance")
	s.fails(2, "ERR_DURATION", "add", "1.5h", "Study")
	s.fails(1, "ERR_NOT_FOUND", "start", "1")
	s.ok("add", "1h", "Study")
	s.fails(1, "ERR_ILLEGAL_TRANSITION", "pause", s.id("Study"))
	s.fails(2, "ERR_PAST_DAY", "--date", "2026-11-16", "add", "1h", "Late")
	s.ok("--date", "2026-11-18", "add", "1h", "Tomorrow")
	s.fails(1, "ERR_DAY_CLOSED", "--date", "2026-11-18", "start", s.id("Tomorrow", "--date", "2026-11-18"))
	s.fails(2, "ERR_USAGE", "--json", "add", "1h")
}

func TestEditDeleteAndEveryAction(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Study")
	if out := s.ok("edit", "--title", "Deep study", "--duration", "2h", s.id("Study")); !contains(out, "│ Deep study │       2h │   TODO │") {
		t.Fatalf("got %q", out)
	}
	for _, step := range []string{"start", "pause", "resume", "finish", "reopen", "skip", "unskip"} {
		s.now = s.now.Add(time.Minute)
		s.ok(step, s.id("Deep study"))
	}
	s.ok("delete", "--yes", s.id("Deep study"))
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
	if code != 2 || !strings.HasPrefix(errOut, "Error: Titles cannot hold hidden or direction-changing characters.\n") || !contains(errOut, "Titles cannot hold hidden or direction-changing characters") || !contains(errOut, "Remove the invisible or direction-changing character U+202E at position 12") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	s.ok("add", "1h", "Ship it 👩\u200d💻 ✔︎")
	s.fails(2, "ERR_TITLE", "edit", "--title", "zero\u200bwidth", s.id("Ship it 👩\u200d💻 ✔\ufe0e"))
	if out := s.ok("doctor"); out != "OK\n" {
		t.Fatalf("a clean database should not warn: %q", out)
	}
}

func TestADurationLongerThanTheDayIsRefused(t *testing.T) {
	s := newSession(t) // 09:00, fifteen hours left
	_, errOut, code := s.run("add", "30h", "Too much")
	if code != 2 || errOut != "Error: Only 15h left on this day.\n       Enter 15h or less, or plan this on another day.\n" {
		t.Fatalf("exit %d: %q", code, errOut)
	}
	s.ok("add", "15h", "Exactly enough")
	s.fails(2, "ERR_DURATION_LIMIT", "add", "1m", "One more")
}

func TestLicensesArePrintedFromTheProgram(t *testing.T) {
	s := newSession(t)
	out := s.ok("--licenses")
	for _, want := range []string{"flowd is built with code from the projects below", "Go standard library", "modernc.org/sqlite", "charm.land/bubbletea", "Permission is hereby granted"} {
		if !contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if _, err := os.Stat(s.db); !os.IsNotExist(err) {
		t.Fatal("--licenses must not touch the database")
	}
}

func TestDeleteAsksAtATerminal(t *testing.T) {
	cases := []struct {
		answer  string
		deleted bool
	}{{"y\n", true}, {"yes\n", true}, {"Y\n", true}, {"n\n", false}, {"\n", false}, {"", false}, {"nope\n", false}}
	for _, c := range cases {
		s := newSession(t)
		s.ok("add", "1h", "Study")
		s.answer = &c.answer
		out := s.ok("delete", s.id("Study"))
		if !contains(out, `Delete "Study"? This cannot be undone. [y/N]`) {
			t.Fatalf("%q: no question in %q", c.answer, out)
		}
		s.answer = nil
		left := !contains(s.ok("list"), "Nothing planned")
		if left == c.deleted || contains(out, "Not deleted") == c.deleted {
			t.Errorf("answer %q: deleted=%v, output %q", c.answer, !left, out)
		}
	}
}

func TestDeleteWithoutATerminalNeedsYes(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Study")
	_, errOut, code := s.run("delete", s.id("Study"))
	if code != 2 || !strings.HasPrefix(errOut, "Error: Deleting needs confirmation.\n       Add --yes") {
		t.Fatalf("exit %d: %q", code, errOut)
	}
	answer := "y\n"
	s.answer = &answer
	s.fails(2, "ERR_USAGE", "--json", "delete", s.id("Study")) // JSON means nobody is reading a question
	s.answer = nil
	if contains(s.ok("list"), "Nothing planned") {
		t.Fatal("a refused delete removed the item")
	}
	var deleted map[string]any
	if err := json.Unmarshal([]byte(s.ok("--json", "delete", "--yes", s.id("Study"))), &deleted); err != nil || deleted["archived"] != true {
		t.Fatalf("got %v, %v", deleted, err)
	}
}

func TestDoctorAcceptsARelativeDatabasePath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	s := &session{t: t, db: "plan.sqlite", now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
	s.ok("add", "1h", "Study")
	if out := s.ok("doctor"); out != "OK\n" {
		t.Fatalf("got %q", out)
	}
}

// Positions are never accepted, only full IDs, so a row number can never
// act on an item that moved.
func TestItemsArePickedOnlyByTheirFullID(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Study")
	id := s.id("Study")
	s.fails(1, "ERR_NOT_FOUND", "start", "1")
	s.fails(1, "ERR_NOT_FOUND", "start", id[:len(id)-1])
	s.fails(1, "ERR_NOT_FOUND", "start", id[3:])
	if out := s.ok("start", id); !contains(out, "Study") || !contains(out, id) {
		t.Fatalf("got %q", out)
	}
	if list := s.ok("list"); contains(list, "#") || !contains(list, id) {
		t.Fatalf("the list should show IDs and no row numbers:\n%s", list)
	}
	if out := s.ok("add", "30m", "Code"); !contains(out, "it_") {
		t.Fatalf("a change should print the item's ID: %q", out)
	}
}

// Options work before the command, after it and after its arguments, but
// each value only once.
func TestOptionsCanGoAnywhere(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Today")
	s.ok("add", "--date", "2026-11-18", "1h", "Tomorrow")
	for _, args := range [][]string{
		{"--date", "2026-11-18", "list"},
		{"list", "--date", "2026-11-18"},
		{"2026-11-18"},
	} {
		if out := s.ok(args...); !contains(out, "Tomorrow") || contains(out, "Today") {
			t.Fatalf("%v: %s", args, out)
		}
	}
	id := s.id("Today")
	if out := s.ok("start", id, "--json"); !strings.HasPrefix(out, "{") {
		t.Fatalf("--json after the ID was ignored: %q", out)
	}
	if out := s.ok("add", "30m", "--", "-v is a flag"); !contains(out, "│ -v is a flag │") {
		t.Fatalf("a title after -- is plain text: %q", out)
	}
	s.fails(2, "ERR_USAGE", "--date", "2026-11-18", "list", "--date", "2026-11-19")
	s.fails(2, "ERR_USAGE", "2026-11-18", "--date", "2026-11-18")
	s.fails(2, "ERR_USAGE", "list", "--colour")
}

// A person never sees an error code, only what went wrong and what to do.
func TestErrorsReadAsSentencesForPeople(t *testing.T) {
	s := newSession(t)
	_, errOut, code := s.run("list", "--colour")
	if code != 2 || errOut != "Error: There is no option --colour.\n       See flowd --help for every command and option.\n" {
		t.Fatalf("exit %d: %q", code, errOut)
	}
	_, errOut, _ = s.run("start", "it_missing")
	if errOut != "Error: No item it_missing on 2026-11-17.\n       Use the full ID shown by flowd list.\n" {
		t.Fatalf("got %q", errOut)
	}
	_, errOut, _ = s.run("--json", "start", "it_missing")
	var e struct {
		Error struct{ Code, Message, Hint string } `json:"error"`
	}
	if err := json.Unmarshal([]byte(errOut), &e); err != nil || e.Error.Code != "ERR_NOT_FOUND" || e.Error.Message != "No item it_missing on 2026-11-17" || e.Error.Hint == "" {
		t.Fatalf("got %q", errOut)
	}
	if _, errOut, _ := s.run("dance", "--json"); jsonCode(t, errOut) != "ERR_USAGE" {
		t.Fatalf("a command line that cannot be read still answers in JSON: %q", errOut)
	}
}

// Every table starts with the day it is about, and never the timezone.
func TestEveryTableNamesItsDay(t *testing.T) {
	s := newSession(t)
	out := s.ok("add", "--date", "2026-11-20", "1h", "Later")
	if !strings.HasPrefix(out, "Fri 20 Nov 2026\n┌") {
		t.Fatalf("add: %q", out)
	}
	s.ok("add", "1h", "Study")
	id := s.id("Study")
	for _, args := range [][]string{{"start", id}, {"now"}, {"pause", id}, {"edit", "--title", "Deep study", id}, {"list"}, {"delete", "--yes", id}} {
		if out := s.ok(args...); !strings.HasPrefix(out, "Tue 17 Nov 2026\n┌") || contains(out, "UTC") {
			t.Fatalf("%v: %q", args, out)
		}
	}
}
