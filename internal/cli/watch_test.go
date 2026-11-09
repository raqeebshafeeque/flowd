package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/raqeebshafeeque/flowd/internal/store"
	"strings"
	"testing"
	"time"
)

func TestWatchFrameModes(t *testing.T) {
	for _, test := range []struct {
		name           string
		terminal, json bool
		controls       bool
	}{
		{"pipe text", false, false, false}, {"terminal text", true, false, true}, {"terminal json", true, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := writeFrame(&out, []byte("{}\n"), test.terminal && !test.json); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "\x1b[") != test.controls {
				t.Error("redraw controls must be terminal text only")
			}
		})
	}
}

func TestWatchClockJumps(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		name := "implicit rollover"
		if explicit {
			name = "explicit date pinned"
		}
		t.Run(name, func(t *testing.T) {
			f := newCommandFixture(t)
			f.now = commandTime(t, "2026-10-14T18:29:59Z")
			var out, stderr bytes.Buffer
			ticks, captures := 0, 0
			app := application{out: &out, now: func() time.Time { captures++; return f.now }, wait: func(context.Context) error {
				ticks++
				switch ticks {
				case 1:
					f.now = commandTime(t, "2026-10-14T21:30:00Z")
				case 2:
					f.now = commandTime(t, "2026-10-14T18:29:00Z")
				case 3:
					return context.Canceled
				}
				return nil
			}}
			args := []string{"--db", f.path, "--timezone", "Asia/Kolkata", "--json"}
			if explicit {
				args = append(args, "--date", "2026-10-14")
			}
			args = append(args, "watch")
			if app.run(context.Background(), args, &stderr, "test") != 0 {
				t.Fatal(stderr.String())
			}
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if len(lines) != 3 || captures != 3 {
				t.Fatal("each refresh should capture the actual clock once, without replaying missed ticks")
			}
			for index, line := range lines {
				var view dayView
				if err := json.Unmarshal([]byte(line), &view); err != nil {
					t.Fatal(err)
				}
				date := "2026-10-14"
				if index == 1 && !explicit {
					date = "2026-10-15"
				}
				if view.Date != date {
					t.Errorf("snapshot %d got date %s, want %s", index, view.Date, date)
				}
			}
		})
	}
}

func TestWatchReleasesAndRetries(t *testing.T) {
	f := newCommandFixture(t)
	var out, stderr bytes.Buffer
	ticks := 0
	var holder *store.Store
	app := application{out: &out, now: func() time.Time { return f.now }, wait: func(ctx context.Context) error {
		ticks++
		switch ticks {
		case 1:
			var err error
			holder, err = store.Open(ctx, f.path)
			if err != nil {
				t.Fatalf("watch held ownership while waiting: %v", err)
			}
		case 2:
			holder.Close()
		case 3:
			return context.Canceled
		}
		return nil
	}}
	if exit := app.run(context.Background(), []string{"--db", f.path, "--json", "watch"}, &stderr, "test"); exit != 0 {
		t.Fatalf("watch failed: %s", stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if ticks != 3 || len(lines) != 2 || stderr.Len() != 0 {
		t.Error("busy refresh should retry without a snapshot or diagnostic flood")
	}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Error("watch must emit one complete JSON object per snapshot")
		}
	}
}
