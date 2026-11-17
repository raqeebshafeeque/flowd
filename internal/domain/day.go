package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidDate = errors.New("invalid date")
	ErrInvalidZone = errors.New("invalid timezone")
	ErrDayClosed   = errors.New("the day is not open")
)

// Day is one planned date. It keeps the timezone it was created in, so
// travelling never moves its midnight.
type Day struct {
	ID        ID         `json:"id"`
	Date      string     `json:"date"`
	Timezone  string     `json:"timezone"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// NewDay creates a day for a date like "2026-11-17" in an IANA timezone such
// as "Asia/Kolkata". "Local" is refused because it would change with the
// machine.
func NewDay(date, timezone string, now time.Time) (Day, error) {
	day := Day{ID: NewID("dy"), Date: date, Timezone: timezone, CreatedAt: Capture(now)}
	if _, _, err := day.Bounds(); err != nil {
		return Day{}, err
	}
	if !ValidInstant(day.CreatedAt) {
		return Day{}, fmt.Errorf("%w: invalid creation time", ErrInvalidEvidence)
	}
	return day, nil
}

// Bounds returns the first instant of the day and the first instant of the
// next one, in UTC. Both are midnight in the day's own timezone, so a day can
// be 23 or 25 hours long around daylight saving changes.
func (d Day) Bounds() (start, end time.Time, err error) {
	date, err := time.Parse("2006-01-02", d.Date)
	if err != nil || date.Format("2006-01-02") != d.Date {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: %q", ErrInvalidDate, d.Date)
	}
	if d.Timezone == "" || d.Timezone == "Local" {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: %q", ErrInvalidZone, d.Timezone)
	}
	zone, err := time.LoadLocation(d.Timezone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: %q", ErrInvalidZone, d.Timezone)
	}
	if start, err = midnight(date, zone); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if end, err = midnight(date.AddDate(0, 0, 1), zone); err != nil {
		return time.Time{}, time.Time{}, err
	}
	return start, end, nil
}

// midnight finds the first instant of a civil date in zone. A date whose
// midnight was skipped by a clock change is refused rather than shifted, and
// a midnight that happens twice starts at its earlier occurrence.
func midnight(date time.Time, zone *time.Location) (time.Time, error) {
	t := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, zone)
	if t.Format("2006-01-02 15:04:05") != date.Format("2006-01-02")+" 00:00:00" || t.UTC().Year() < 1 || t.UTC().Year() > 9999 {
		return time.Time{}, fmt.Errorf("%w: %s has no midnight in %s", ErrInvalidDate, date.Format("2006-01-02"), zone)
	}
	_, offset := t.Zone()
	for h := -48; h <= 48; h++ {
		_, other := t.Add(time.Duration(h) * time.Hour).Zone()
		candidate := t.Add(time.Duration(offset-other) * time.Second)
		if candidate.Before(t) && candidate.In(zone).Format("2006-01-02 15:04:05") == t.Format("2006-01-02 15:04:05") {
			t = candidate
			_, offset = t.Zone()
		}
	}
	return t.UTC(), nil
}

// RequireOpen refuses unless now falls inside the day. Midnight itself
// already belongs to the next day.
func (d Day) RequireOpen(now time.Time) error {
	start, end, err := d.Bounds()
	if err != nil {
		return err
	}
	if !ValidInstant(now) || now.Before(start) || !now.Before(end) {
		return fmt.Errorf("%w: %s", ErrDayClosed, d.Date)
	}
	return nil
}
