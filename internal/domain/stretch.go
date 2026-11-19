package domain

import "time"

// Stretch is one unbroken span of an item's history: running or paused.
type Stretch struct {
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Running bool      `json:"running"`
}

// Breaks counts how a history stopped work before it was done. A switch is
// a pause because another item started, and a stop is a pause someone asked
// for. Finishing or skipping ends the work and is neither.
type Breaks struct {
	Switches int `json:"switches"`
	Stops    int `json:"stops"`
}

// Total is every interruption, switches and stops together.
func (b Breaks) Total() int { return b.Switches + b.Stops }

// Stretches lays an item's history out as running and paused stretches,
// up to observed and never past the day's midnight, and counts the breaks.
// It expects history that Measure accepts, so call Measure first.
func Stretches(day Day, events []Event, observed time.Time) ([]Stretch, Breaks) {
	var out []Stretch
	var breaks Breaks
	_, cutoff, err := day.Bounds()
	if err != nil {
		return nil, breaks
	}
	observed = Capture(observed)
	if observed.After(cutoff) {
		observed = cutoff
	}
	var open *Stretch
	close := func(at time.Time) {
		if open != nil {
			if at.After(open.From) {
				open.To = at
				out = append(out, *open)
			}
			open = nil
		}
	}
	for _, e := range events {
		if e.OccurredAt.After(observed) {
			break
		}
		state := e.After.Status
		if e.After.Archived {
			state = ""
		}
		if e.Before != nil && e.Before.Status == InProgress && state == Paused {
			if e.Reason == ReasonAutoPause {
				breaks.Switches++
			} else {
				breaks.Stops++
			}
		}
		switch {
		case state == InProgress && (open == nil || !open.Running):
			close(e.OccurredAt)
			open = &Stretch{From: e.OccurredAt, Running: true}
		case state == Paused && (open == nil || open.Running):
			close(e.OccurredAt)
			open = &Stretch{From: e.OccurredAt}
		case state != InProgress && state != Paused:
			close(e.OccurredAt)
		}
	}
	close(observed)
	return out, breaks
}
