package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// tab runs the hidden __complete command the way a shell script does, with
// the words typed after flowd and the word being completed last.
func (s *session) tab(words ...string) string {
	s.t.Helper()
	var out bytes.Buffer
	env := Env{Now: func() time.Time { return s.now }, Zone: time.UTC}
	if code := Run(context.Background(), append([]string{"__complete", "--db", s.db}, words...), &out, &bytes.Buffer{}, env); code != 0 {
		s.t.Fatalf("exit %d", code)
	}
	return out.String()
}

func TestCompletionOffersOnlyItemsTheCommandFits(t *testing.T) {
	s := newSession(t)
	s.ok("add", "1h", "Deep work")
	s.ok("add", "30m", "Inbox")
	s.ok("add", "10m", "Gone")
	deep, inbox := s.id("Deep work"), s.id("Inbox")
	s.ok("delete", "--yes", s.id("Gone"))
	s.ok("start", deep)
	cases := map[string]string{
		"start":  inbox + "\tInbox\n",
		"pause":  deep + "\tDeep work\n",
		"finish": deep + "\tDeep work\n",
		"edit":   deep + "\tDeep work\n" + inbox + "\tInbox\n",
	}
	for command, want := range cases {
		if got := s.tab(command, ""); got != want {
			t.Errorf("%s: got %q, want %q", command, got, want)
		}
	}
	if got := s.tab("start", inbox, ""); got != "" {
		t.Errorf("only one ID is taken: %q", got)
	}
	if got := s.tab("start", inbox[:5]); !strings.Contains(got, inbox) {
		t.Errorf("a partial ID should complete: %q", got)
	}
}

func TestCompletionFollowsTheDateAndCompletesWords(t *testing.T) {
	s := newSession(t)
	s.ok("add", "--date", "2026-11-20", "1h", "Later")
	if got := s.tab("--date", "2026-11-20", "edit", ""); !strings.Contains(got, "\tLater\n") {
		t.Errorf("--date should pick the day: %q", got)
	}
	if got := s.tab("edit", "--date=2026-11-20", ""); !strings.Contains(got, "\tLater\n") {
		t.Errorf("--date= should pick the day: %q", got)
	}
	if got := s.tab("edit", ""); got != "" {
		t.Errorf("today has nothing planned: %q", got)
	}
	if got := s.tab("fin"); got != "finish\tMark an item done for today\n" {
		t.Errorf("commands: %q", got)
	}
	if got := s.tab("now", "--s"); got != "--short\tOne line for a status bar\n" {
		t.Errorf("options: %q", got)
	}
	if got := s.tab("completion", ""); got != "bash\nfish\nzsh\n" {
		t.Errorf("shells: %q", got)
	}
}

// Completion runs on every press of Tab, so like the status line it takes no
// lock, creates nothing and stays quiet on any problem.
func TestCompletionNeverGetsInTheWay(t *testing.T) {
	s := newSession(t)
	if got := s.tab("start", ""); got != "" {
		t.Fatalf("got %q", got)
	}
	if _, err := os.Stat(s.db); err == nil {
		t.Fatal("completion must not create the database")
	}
	os.WriteFile(s.db, []byte("not a database at all, just some text"), 0o600)
	if got := s.tab("start", ""); got != "" {
		t.Fatalf("broken: %q", got)
	}
}

func TestEachShellGetsItsScript(t *testing.T) {
	s := newSession(t)
	for shell, want := range map[string]string{"fish": "complete -c flowd", "zsh": "#compdef flowd", "bash": "complete -o nosort -F _flowd flowd"} {
		if out := s.ok("completion", shell); !strings.Contains(out, want) || !strings.Contains(out, "flowd __complete") {
			t.Errorf("%s: %q", shell, out)
		}
	}
	s.fails(2, "ERR_USAGE", "completion", "powershell")
	s.fails(2, "ERR_USAGE", "completion")
}

// A title can never break the one line per candidate format.
func TestATitleStaysOnOneLine(t *testing.T) {
	if got := oneLine("a\tb\nc"); got != "a b c" {
		t.Fatalf("got %q", got)
	}
}
