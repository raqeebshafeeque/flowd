package planner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// Period is how much a review covers.
type Period string

const (
	DayPeriod   Period = "day"
	WeekPeriod  Period = "week"
	MonthPeriod Period = "month"
)

// ItemInsight is an item with its running and paused stretches, for the
// timeline of a day.
type ItemInsight struct {
	ItemView
	Stretches []domain.Stretch `json:"stretches"`
}

// DayInsight is how one day really went. Everything is worked out from the
// saved history, so it always agrees with flowd list.
type DayInsight struct {
	Date  string        `json:"date"`
	Items []ItemInsight `json:"items"`
	// FocusedNS is the time items ran, and PausedNS the time started items
	// waited. Sessions counts unbroken running stretches, and LongestNS is
	// the longest of them.
	FocusedNS int64         `json:"focused_ns"`
	PausedNS  int64         `json:"paused_ns"`
	Sessions  int           `json:"sessions"`
	LongestNS int64         `json:"longest_ns"`
	Breaks    domain.Breaks `json:"interruptions"`
	// PlannedMinutes adds up the durations of the day's items, and Finished
	// counts the done ones out of Total.
	PlannedMinutes int64 `json:"planned_minutes"`
	Finished       int   `json:"finished"`
	Total          int   `json:"total"`
}

// Active tells whether any work happened on the day.
func (d DayInsight) Active() bool { return d.FocusedNS > 0 }

// Insight measures a day from its view and history at the instant now.
func Insight(day domain.Day, view View, history map[domain.ID][]domain.Event, now time.Time) DayInsight {
	d := DayInsight{Date: view.Date, Items: []ItemInsight{}}
	for _, item := range view.Items {
		stretches, breaks := domain.Stretches(day, history[item.ID], now)
		if stretches == nil {
			stretches = []domain.Stretch{}
		}
		d.Items = append(d.Items, ItemInsight{item, stretches})
		d.FocusedNS += item.WorkNS
		if item.PausedNS != nil {
			d.PausedNS += *item.PausedNS
		}
		for _, s := range stretches {
			if s.Running {
				d.Sessions++
				d.LongestNS = max(d.LongestNS, s.To.Sub(s.From).Nanoseconds())
			}
		}
		d.Breaks.Switches += breaks.Switches
		d.Breaks.Stops += breaks.Stops
		d.PlannedMinutes += item.DurationMinutes
		d.Total++
		if item.Status == domain.Done {
			d.Finished++
		}
	}
	return d
}

// PeriodInsight is a review of every date from From to To, both included.
// A date nothing was planned on is there too, empty.
type PeriodInsight struct {
	Period Period       `json:"period"`
	From   string       `json:"from"`
	To     string       `json:"to"`
	Today  string       `json:"today"`
	Days   []DayInsight `json:"days"`
}

// Sum adds up the days of a period. Longest is the longest single session,
// and LongestDate the day it happened on.
func (p PeriodInsight) Sum() (total DayInsight, longestDate string, active int) {
	for _, d := range p.Days {
		total.FocusedNS += d.FocusedNS
		total.PausedNS += d.PausedNS
		total.Sessions += d.Sessions
		total.Breaks.Switches += d.Breaks.Switches
		total.Breaks.Stops += d.Breaks.Stops
		total.PlannedMinutes += d.PlannedMinutes
		total.Finished += d.Finished
		total.Total += d.Total
		if d.LongestNS > total.LongestNS {
			total.LongestNS, longestDate = d.LongestNS, d.Date
		}
		if d.Active() {
			active++
		}
	}
	return total, longestDate, active
}

// Range is the first and last date of the period around date: the date
// itself, its week from Monday to Sunday, or its calendar month.
func Range(period Period, date string) (from, to string, err error) {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", "", err
	}
	switch period {
	case DayPeriod:
		return date, date, nil
	case WeekPeriod:
		monday := d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
		return monday.Format("2006-01-02"), monday.AddDate(0, 0, 6).Format("2006-01-02"), nil
	case MonthPeriod:
		first := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)
		return first.Format("2006-01-02"), first.AddDate(0, 1, -1).Format("2006-01-02"), nil
	}
	return "", "", fmt.Errorf("unknown period %q", period)
}

// Review reads a whole period in one open of the database: today's when
// date is empty. Like every operation it closes due days first.
func (c Client) Review(ctx context.Context, period Period, date string) (p PeriodInsight, err error) {
	s, err := c.open(ctx, Selection{Date: date})
	if err != nil {
		return PeriodInsight{}, err
	}
	defer func() { err = errors.Join(err, s.db.Close()) }()
	from, to, err := Range(period, s.day.Date)
	if err != nil {
		return PeriodInsight{}, err
	}
	p = PeriodInsight{Period: period, From: from, To: to, Today: s.now.In(s.zone).Format("2006-01-02"), Days: []DayInsight{}}
	start, _ := time.Parse("2006-01-02", from)
	end, _ := time.Parse("2006-01-02", to)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		date := d.Format("2006-01-02")
		day, err := s.db.DayByDate(ctx, date)
		if errors.Is(err, store.ErrNotFound) {
			p.Days = append(p.Days, DayInsight{Date: date, Items: []ItemInsight{}})
			continue
		}
		if err != nil {
			return PeriodInsight{}, err
		}
		items, err := s.db.Items(ctx, day.ID)
		if err != nil {
			return PeriodInsight{}, err
		}
		history, err := s.db.DayEvents(ctx, day.ID)
		if err != nil {
			return PeriodInsight{}, err
		}
		view, err := NewView(day, items, history, s.now)
		if err != nil {
			return PeriodInsight{}, err
		}
		p.Days = append(p.Days, Insight(day, view, history, s.now))
	}
	return p, nil
}
