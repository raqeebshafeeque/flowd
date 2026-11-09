package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
	"github.com/raqeebshafeeque/flowd/internal/utils"
)

// This representative fixture records observations, not machine-independent targets.
func TestRepresentativeHistory(t *testing.T) {
	f := newCommandFixture(t)
	ctx := context.Background()
	database, err := store.Open(ctx, f.path)
	if err != nil {
		t.Fatal(err)
	}
	base := commandTime(t, "2024-10-15T00:00:00Z")
	count := 0
	started := time.Now()
	for index := range 730 {
		date := base.AddDate(0, 0, index)
		day := domain.Day{ID: utils.ID(fmt.Sprintf("dy_%03d", index)), Date: date.Format("2006-01-02"), Timezone: "UTC", CreatedAt: date}
		amount := 2
		if index < 540 {
			amount = 3
		}
		for slot := range amount {
			block := domain.Block{ID: utils.ID(fmt.Sprintf("bl_%04d", count)), DayID: day.ID, Title: "Representative work", CreatedAt: date, Status: domain.Todo, PlannedStart: date.Add(time.Duration(9+2*slot) * time.Hour), PlannedEnd: date.Add(time.Duration(10+2*slot) * time.Hour)}
			event := eventFor(block, block.Status, date, domain.ReasonUser)
			if slot == 0 {
				err = database.CreateDayWithBlock(ctx, day, block, event)
			} else {
				err = database.CreateBlock(ctx, block, event)
			}
			if err != nil {
				t.Fatal(err)
			}
			count++
		}
	}
	creation := time.Since(started)
	total := 0
	for index := range 730 {
		blocks, err := database.BlocksByDay(ctx, utils.ID(fmt.Sprintf("dy_%03d", index)))
		if err != nil {
			t.Fatal(err)
		}
		total += len(blocks)
	}
	if total != 2000 || count != 2000 {
		t.Errorf("got %d blocks, want 2000", total)
	}
	database.Close()
	started = time.Now()
	if err := store.Inspect(ctx, f.path); err != nil {
		t.Fatal(err)
	}
	inspection := time.Since(started)
	var elapsed, max time.Duration
	app := application{now: func() time.Time { return f.now }}
	for range 20 {
		var out, stderr bytes.Buffer
		app.out = &out
		started = time.Now()
		if exit := app.run(ctx, []string{"--db", f.path, "--date", "2026-10-14", "--json", "list"}, &stderr, "test"); exit != 0 {
			t.Fatal(stderr.String())
		}
		duration := time.Since(started)
		elapsed += duration
		if duration > max {
			max = duration
		}
		if out.Len() == 0 {
			t.Error("representative snapshot is empty")
		}
	}
	if binary := os.Getenv("FLOWD_MEASURE_BINARY"); binary != "" {
		var sum, slowest time.Duration
		for range 20 {
			started := time.Now()
			output, err := exec.Command(binary, "--db", f.path, "--date", "2026-10-14", "--json", "list").CombinedOutput()
			if err != nil {
				t.Fatalf("measured binary failed: %v; %s", err, output)
			}
			duration := time.Since(started)
			sum += duration
			if duration > slowest {
				slowest = duration
			}
		}
		t.Logf("built-process snapshots: mean=%s max=%s (20 runs; no cold-cache claim)", sum/20, slowest)
	}
	info, err := os.Stat(f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("730 days / 2000 blocks: creation=%s full-inspection=%s snapshot-mean=%s snapshot-max=%s database-bytes=%d", creation, inspection, elapsed/20, max, info.Size())
}
