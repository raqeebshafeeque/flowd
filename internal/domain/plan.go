package domain

import (
	"errors"
	"fmt"
)

var ErrActiveBlock = errors.New("another block is already running")

// ValidatePlan checks the whole resulting visible plan, not intermediate moves.
func ValidatePlan(blocks []Block) error {
	running := 0
	for index, block := range blocks {
		if block.Archived {
			continue
		}
		if err := (NewBlockInput{Title: block.Title, DayID: block.DayID, PlannedStartAt: block.PlannedStart, PlannedEndAt: block.PlannedEnd}).Validate(); err != nil {
			return err
		}
		if block.Status == InProgress {
			running++
		}
		if IsOverlapping(block.PlannedStart, block.PlannedEnd, blocks[index+1:]) {
			return ErrOverlap
		}
	}
	if running > 1 {
		return fmt.Errorf("%w: at most one per day", ErrActiveBlock)
	}
	return nil
}
