package domain

import (
	"fmt"
	"time"
)

// A day is a fixed amount of time, so the Durations planned on it cannot add
// up to more than what is left of it. These functions work out how much is
// left. They never change saved items: only new or larger Durations are
// checked against them.

// Reservation is what another item of the day still holds back: its status,
// its Duration and the work already recorded on it.
type Reservation struct {
	Status          Status
	DurationMinutes int64
	WorkNS          int64
	Archived        bool
}

// ReservedMinutes is how much of the day an item still holds. A todo item
// holds its full Duration. A running or paused one holds what it has not
// worked yet, counting a partly worked minute as a whole one. Done,
// incomplete and deleted items hold nothing.
func (r Reservation) ReservedMinutes() int64 {
	if r.Archived {
		return 0
	}
	switch r.Status {
	case Todo:
		return r.DurationMinutes
	case InProgress, Paused:
		left := r.DurationMinutes*int64(time.Minute) - r.WorkNS
		if left <= 0 {
			return 0
		}
		return (left + int64(time.Minute) - 1) / int64(time.Minute)
	}
	return 0
}

// RemainingMinutes is the whole minutes a new or larger Duration may use on
// day: the time from now (or from the start of a day that has not begun) to
// the day's midnight, minus what the other items hold. It is never negative.
func RemainingMinutes(day Day, now time.Time, others []Reservation) (int64, error) {
	start, end, err := day.Bounds()
	if err != nil {
		return 0, err
	}
	if !ValidInstant(now) {
		return 0, fmt.Errorf("%w: invalid time", ErrInvalidEvidence)
	}
	from := Capture(now)
	if from.Before(start) {
		from = start
	}
	if !from.Before(end) {
		return 0, nil
	}
	left := int64(end.Sub(from) / time.Minute)
	for _, other := range others {
		reserved := other.ReservedMinutes()
		if reserved >= left {
			return 0, nil
		}
		left -= reserved
	}
	return left, nil
}
