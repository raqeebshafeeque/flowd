package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/raqeebshafeeque/flowd/internal/utils"
)

type Status string

const (
	Todo       Status = "todo"
	InProgress Status = "in_progress"
	Paused     Status = "paused"
	Done       Status = "done"
	Skipped    Status = "skipped"
)

var (
	ErrIllegalTransition  = errors.New("illegal status transition")
	ErrInvalidBlockTitle  = errors.New("invalid block title")
	ErrInvalidStartAndEnd = errors.New("invalid start and end")
)

type Block struct {
	ID        utils.ID   `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`

	DayID  utils.ID `json:"day_id"`
	Title  string   `json:"title"`
	Status Status   `json:"status"`

	PlannedStart time.Time  `json:"planned_start"`
	PlannedEnd   time.Time  `json:"planned_end"`
	ActualStart  *time.Time `json:"actual_start"`
	ActualEnd    *time.Time `json:"actual_end"`

	Archived bool `json:"archived"`

	// The block from which this block is created
	CopiedFromID *utils.ID `json:"copied_from_id"`
}

type NewBlockInput struct {
	Title          string
	DayID          utils.ID
	PlannedStartAt time.Time
	PlannedEndAt   time.Time
	CopiedFromID   *utils.ID
}

func (inp NewBlockInput) Validate() error {
	if strings.TrimSpace(inp.Title) == "" || utf8.RuneCountInString(inp.Title) > 120 {
		return fmt.Errorf("%w: title: %s | title length : %d", ErrInvalidBlockTitle, inp.Title, len(inp.Title))
	}

	if inp.PlannedStartAt.Equal(inp.PlannedEndAt) || inp.PlannedStartAt.After(inp.PlannedEndAt) {
		return fmt.Errorf("%w: start at: %s | end at : %s", ErrInvalidStartAndEnd, inp.PlannedStartAt, inp.PlannedEndAt)
	}

	return nil
}

// NewBlock creates a new block instance based on the input
func NewBlock(input NewBlockInput) (*Block, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	block := Block{
		ID:           utils.NewIDWithPrefix("bl"),
		CreatedAt:    time.Now().UTC(),
		Title:        strings.TrimSpace(input.Title),
		DayID:        input.DayID,
		PlannedStart: input.PlannedStartAt,
		PlannedEnd:   input.PlannedEndAt,
		Status:       Todo,
		CopiedFromID: input.CopiedFromID,
	}

	return &block, nil
}

// UpdateStatus moves the block to a new status when the lifecycle table allows
// it, stamping actual times from now. It reports ErrIllegalTransition otherwise.
func (b *Block) UpdateStatus(to Status, now time.Time) error {
	switch b.Status {
	case Todo:
		switch to {
		case InProgress:
			if b.ActualStart == nil {
				stamp := now
				b.ActualStart = &stamp
			}
			b.Status = InProgress
			return nil
		case Skipped:
			b.Status = Skipped
			return nil
		}
	case InProgress:
		switch to {
		case Paused, Skipped:
			b.Status = to
			return nil
		case Done:
			endedAt := now
			b.ActualEnd = &endedAt
			b.Status = Done
			return nil
		}
	case Paused:
		switch to {
		case InProgress, Skipped:
			b.Status = to
			return nil
		case Done:
			endedAt := now
			b.ActualEnd = &endedAt
			b.Status = Done
			return nil
		}
	}

	// TODO: Reopen from done stays refused until the Day type arrives with the calendar
	// date the same day rule needs.

	return fmt.Errorf("%w: %q to %q", ErrIllegalTransition, b.Status, to)
}

// IsOverlapping checks if the planned time overlap with any other block
// under the same day.
func IsOverlapping(startAt, endAt time.Time, existingBlocks []Block) bool {
	for _, ob := range existingBlocks {
		// archived blocks are inactive, hence skip
		if ob.Archived {
			continue
		}

		if !ob.PlannedEnd.After(startAt) ||
			!endAt.After(ob.PlannedStart) {
			continue
		}
		return true
	}
	return false
}
