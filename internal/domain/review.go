package domain

import (
	"fmt"
	"github.com/raqeebshafeeque/flowd/internal/utils"
	"time"
)

// Review describes the current plan without changing stored statuses.
type Review struct {
	Done       int        `json:"done"`
	Skipped    int        `json:"skipped"`
	Unfinished int        `json:"unfinished"`
	Missed     []utils.ID `json:"missed"`
	Overruns   []utils.ID `json:"overruns"`
	Progress   *int       `json:"progress"`
}

func ReviewDay(day Day, blocks []Block, now time.Time) (Review, error) {
	result := Review{Missed: make([]utils.ID, 0), Overruns: make([]utils.ID, 0)}
	zone, err := time.LoadLocation(day.Timezone)
	if err != nil || day.Timezone == "" || day.Timezone == "Local" {
		return result, fmt.Errorf("%w: %q", ErrInvalidDayZone, day.Timezone)
	}
	ended := now.In(zone).Format("2006-01-02") > day.Date
	if score, ok := Progress(blocks); ok {
		result.Progress = &score
	}
	for _, block := range blocks {
		if block.Archived {
			continue
		}
		switch block.Status {
		case Done:
			result.Done++
		case Skipped:
			result.Skipped++
		default:
			result.Unfinished++
		}
		if ended && block.Status == Todo {
			result.Missed = append(result.Missed, block.ID)
		}
		if IsOverrun(block, now) {
			result.Overruns = append(result.Overruns, block.ID)
		}
	}
	return result, nil
}

func IsOverrun(block Block, now time.Time) bool {
	if block.Archived {
		return false
	}
	if block.Status == InProgress || block.Status == Paused {
		return now.After(block.PlannedEnd)
	}
	return block.Status == Done && block.ActualEnd != nil && block.ActualEnd.After(block.PlannedEnd)
}
