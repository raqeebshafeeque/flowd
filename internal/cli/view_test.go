package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
	"github.com/raqeebshafeeque/flowd/internal/utils"
)

type commandFixture struct {
	t    *testing.T
	path string
	now  time.Time
}

func newCommandFixture(t *testing.T) *commandFixture {
	t.Helper()
	t.Setenv("TZ", "UTC")
	return &commandFixture{t: t, path: filepath.Join(t.TempDir(), "flowd.sqlite"), now: commandTime(t, "2026-10-14T09:15:00Z")}
}
func commandTime(t *testing.T, value string) time.Time {
	t.Helper()
	stamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return stamp
}
func (f *commandFixture) run(args ...string) (string, string, int) {
	f.t.Helper()
	var out, stderr bytes.Buffer
	app := application{out: &out, now: func() time.Time { return f.now }}
	exit := app.run(context.Background(), append([]string{"--db", f.path}, args...), &stderr, "test")
	return out.String(), stderr.String(), exit
}
func (f *commandFixture) ok(args ...string) string {
	f.t.Helper()
	out, stderr, exit := f.run(args...)
	if exit != 0 || stderr != "" {
		f.t.Fatalf("command %q exited %d: %s", strings.Join(args, " "), exit, stderr)
	}
	return out
}
func (f *commandFixture) fail(code string, exit int, args ...string) {
	f.t.Helper()
	out, stderr, got := f.run(args...)
	if got != exit || out != "" || !strings.HasPrefix(stderr, code+":") {
		f.t.Fatalf("got exit %d stdout %q stderr %q, want %s and %d", got, out, stderr, code, exit)
	}
}
func (f *commandFixture) seed(date, zone string, blocks ...domain.Block) domain.Day {
	f.t.Helper()
	db, err := store.Open(context.Background(), f.path)
	if err != nil {
		f.t.Fatal(err)
	}
	defer db.Close()
	day := domain.Day{ID: utils.ID("dy_" + date), Date: date, Timezone: zone, CreatedAt: f.now}
	if err := db.CreateDay(context.Background(), day); err != nil {
		f.t.Fatal(err)
	}
	for _, block := range blocks {
		block.DayID = day.ID
		event := domain.BlockEvent{BlockID: block.ID, FromStatus: block.Status, ToStatus: block.Status, OccurredAt: f.now, Reason: domain.ReasonUser}
		if err := db.CreateBlock(context.Background(), block, event); err != nil {
			f.t.Fatal(err)
		}
	}
	return day
}
func (f *commandFixture) blockFixture(id utils.ID, start, end string) domain.Block {
	f.t.Helper()
	return domain.Block{ID: id, Title: string(id), Status: domain.Todo, CreatedAt: f.now,
		PlannedStart: commandTime(f.t, "2026-10-14T"+start+":00Z"), PlannedEnd: commandTime(f.t, "2026-10-14T"+end+":00Z")}
}

func TestEmptyDayViews(t *testing.T) {
	f := newCommandFixture(t)
	var view struct {
		Date, Timezone string
		Blocks         []domain.Block
		Now, Next      *domain.Block
		Progress       *int
	}
	out := f.ok("--json", "list")
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatal(err)
	}
	if view.Date != "2026-10-14" || view.Timezone != "UTC" || view.Blocks == nil || len(view.Blocks) != 0 || view.Now != nil || view.Next != nil || view.Progress != nil {
		t.Error("empty day should have explicit empty/null values and local metadata")
	}
	if out := f.ok("2026-10-15"); !strings.Contains(out, "2026-10-15") || !strings.Contains(out, "no blocks") {
		t.Error("positional date should render an empty day")
	}
	if out := f.ok("--json", "now"); out != "null\n" {
		t.Errorf("got now %q, want null", out)
	}
	db, err := store.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.DayByDate(context.Background(), "2026-10-14"); !errors.Is(err, store.ErrDayNotFound) {
		t.Error("reads must not create day rows")
	}
}

func TestSavedDayViews(t *testing.T) {
	f := newCommandFixture(t)
	first := f.blockFixture("bl_first", "09:00", "10:00")
	first.Title = "Study\x1b[31m\nnotes"
	second := f.blockFixture("bl_second", "11:00", "12:00")
	hidden := f.blockFixture("bl_hidden", "08:00", "09:00")
	hidden.Archived = true
	f.seed("2026-10-14", "UTC", second, hidden, first)
	var current, next domain.Block
	if err := json.Unmarshal([]byte(f.ok("--json", "now")), &current); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(f.ok("--json", "next")), &next); err != nil {
		t.Fatal(err)
	}
	if current.ID != first.ID || next.ID != second.ID {
		t.Error("now and next should follow the selected plan")
	}
	out := f.ok("list")
	if strings.Contains(out, "\x1b") || strings.Contains(out, "bl_hidden") || !strings.Contains(out, "09:00-10:00") {
		t.Error("text should show visible local times without terminal injection")
	}
}

func TestSavedTimezoneSelection(t *testing.T) {
	f := newCommandFixture(t)
	f.now = commandTime(t, "2026-10-13T20:00:00Z")
	f.seed("2026-10-14", "Asia/Kolkata")
	if out := f.ok("--date", "2026-10-14", "list"); !strings.Contains(out, "Asia/Kolkata") {
		t.Error("saved zone should win over the detected system zone")
	}
	f.fail("ERR_USAGE", 2, "--date", "2026-10-14", "--timezone", "UTC", "list")
	f.fail("ERR_USAGE", 2, "--timezone", "not/a-zone", "list")
}
