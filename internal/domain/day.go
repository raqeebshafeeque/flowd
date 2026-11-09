package domain

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/utils"
)

var (
	ErrInvalidDayDate = errors.New("invalid day date")
	ErrInvalidDayZone = errors.New("invalid day timezone")
)

type Day struct {
	ID        utils.ID   `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`

	// Date is the local calendar day in YYYY-MM-DD.
	Date string `json:"date"`
	// Timezone is the IANA name shown on screen, like Asia/Kolkata.
	Timezone string `json:"timezone"`
}

// NewDay builds the day where blocks belong. Date must read YYYY-MM-DD
// and the timezone must load, else the day cannot place blocks on a clock.
func NewDay(date, timezone string) (*Day, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, fmt.Errorf("%w: %q", ErrInvalidDayDate, date)
	}
	if timezone == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidDayZone)
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, fmt.Errorf("%w: %q", ErrInvalidDayZone, timezone)
	}
	return &Day{
		ID:        utils.NewIDWithPrefix("dy"),
		CreatedAt: time.Now().UTC(),
		Date:      date,
		Timezone:  timezone,
	}, nil
}

// Progress says how much of the day got done, as a whole percent. False means
// nothing countable, show a dash instead of a number.
func Progress(blocks []Block) (int, bool) {
	var done, visible, skipped int
	for _, b := range blocks {
		if b.Archived {
			continue
		}
		visible++
		if b.Status == Skipped {
			skipped++
			continue
		}
		if b.Status == Done {
			done++
		}
	}
	denom := visible - skipped
	if denom <= 0 {
		return 0, false
	}
	return int(math.Round(float64(done) / float64(denom) * 100)), true
}
