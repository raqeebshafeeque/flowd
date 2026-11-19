package planner

import (
	"testing"
	"time"
)

// The example day from the requirements: ten minutes running, twenty
// paused, five more running, and a second item that starts and switches.
func TestADayInsightAddsUp(t *testing.T) {
	f := newFixture(t)
	study := f.add("Study", "1h").Item.ID
	code := f.add("Code", "30m").Item.ID
	f.must(Start, study)
	f.at("09:10:00")
	f.must(Pause, study)
	f.at("09:30:00")
	f.must(Resume, study)
	f.at("09:35:00")
	f.must(Start, code) // switches away from Study
	f.at("09:50:00")
	f.must(Finish, code)
	f.at("10:00:00")
	review, err := f.client.Review(ctx, DayPeriod, "")
	if err != nil {
		t.Fatal(err)
	}
	d := review.Days[0]
	if d.FocusedNS != int64(30*time.Minute) || d.Sessions != 3 || d.LongestNS != int64(15*time.Minute) {
		t.Fatalf("focus: %v in %d sessions, longest %v", time.Duration(d.FocusedNS), d.Sessions, time.Duration(d.LongestNS))
	}
	if d.PausedNS != int64(45*time.Minute) {
		t.Fatalf("paused %v, want 20m and then 25m since the switch", time.Duration(d.PausedNS))
	}
	if d.Breaks.Stops != 1 || d.Breaks.Switches != 1 {
		t.Fatalf("breaks %+v", d.Breaks)
	}
	if d.Finished != 1 || d.Total != 2 || d.PlannedMinutes != 90 {
		t.Fatalf("finished %d of %d, planned %d", d.Finished, d.Total, d.PlannedMinutes)
	}
	if len(d.Items) != 2 || len(d.Items[0].Stretches) != 4 {
		t.Fatalf("items %+v", d.Items)
	}
}

func TestPeriodsRunMondayToSundayAndWholeMonths(t *testing.T) {
	cases := []struct {
		period   Period
		date     string
		from, to string
	}{
		{WeekPeriod, "2026-11-19", "2026-11-16", "2026-11-22"},
		{WeekPeriod, "2026-11-16", "2026-11-16", "2026-11-22"},
		{WeekPeriod, "2026-11-22", "2026-11-16", "2026-11-22"},
		{MonthPeriod, "2026-11-19", "2026-11-01", "2026-11-30"},
		{MonthPeriod, "2028-02-10", "2028-02-01", "2028-02-29"},
		{DayPeriod, "2026-11-19", "2026-11-19", "2026-11-19"},
	}
	for _, c := range cases {
		from, to, err := Range(c.period, c.date)
		if err != nil || from != c.from || to != c.to {
			t.Errorf("%s %s: %s to %s, %v", c.period, c.date, from, to, err)
		}
	}
}

// A week lists every one of its days, planned or not, and adds them up.
func TestAWeekHasEveryDayAndItsTotals(t *testing.T) {
	f := newFixture(t)
	a := f.add("Study", "1h").Item.ID
	f.must(Start, a)
	f.at("10:00:00")
	f.must(Finish, a)
	f.now = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	b := f.add("Code", "2h").Item.ID
	f.must(Start, b)
	f.now = f.now.Add(2 * time.Hour)
	week, err := f.client.Review(ctx, WeekPeriod, "")
	if err != nil {
		t.Fatal(err)
	}
	if week.From != "2026-11-16" || week.To != "2026-11-22" || len(week.Days) != 7 || week.Today != "2026-11-18" {
		t.Fatalf("week %s to %s with %d days, today %s", week.From, week.To, len(week.Days), week.Today)
	}
	total, longest, active := week.Sum()
	if total.FocusedNS != int64(3*time.Hour) || active != 2 || longest != "2026-11-18" || total.Finished != 1 || total.Total != 2 {
		t.Fatalf("total %+v, longest on %s, %d active", total, longest, active)
	}
	if week.Days[0].Total != 0 || week.Days[0].Items == nil {
		t.Fatal("a day nothing was planned on is there, empty")
	}
}

func TestAMonthWithNothingPlannedStillHasEveryDay(t *testing.T) {
	f := newFixture(t)
	month, err := f.client.Review(ctx, MonthPeriod, "2028-02-10")
	if err != nil || len(month.Days) != 29 || month.From != "2028-02-01" {
		t.Fatalf("%d days from %s, %v", len(month.Days), month.From, err)
	}
	if total, _, active := month.Sum(); total.FocusedNS != 0 || active != 0 {
		t.Fatalf("got %+v, %d active", total, active)
	}
}

// A month of busy days reviews in well under the budget. It only fails if
// reading a month starts doing much more than one open of the database.
func TestAMonthReviewIsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("timing budgets only run without -short")
	}
	f := newFixture(t)
	for day := 1; day <= 17; day++ {
		f.now = time.Date(2026, 11, day, 8, 0, 0, 0, time.UTC)
		for i := 0; i < 6; i++ {
			id := f.add("Item", "30m").Item.ID
			f.must(Start, id)
			f.now = f.now.Add(20 * time.Minute)
			f.must(Finish, id)
		}
	}
	start := time.Now()
	for i := 0; i < 10; i++ {
		if _, err := f.client.Review(ctx, MonthPeriod, ""); err != nil {
			t.Fatal(err)
		}
	}
	if per := time.Since(start) / 10; per > 200*time.Millisecond {
		t.Errorf("a month review took %v, budget 200ms", per)
	}
}
