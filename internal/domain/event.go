package domain

import (
	"time"

	"github.com/raqeebshafeeque/flowd/internal/utils"
)

// EventReason names what caused a diary row.
type EventReason string

const (
	ReasonUser      EventReason = "user"
	ReasonAutoPause EventReason = "auto_pause"
	ReasonCarryover EventReason = "carryover"
	ReasonReopen    EventReason = "reopen"
	ReasonExtend    EventReason = "extend"
)

// BlockEvent is one line in a block's diary. Rows only grow; nothing edits or
// removes them. The store writes exactly one per block change inside the same
// transaction, so the diary never disagrees with the blocks.
type BlockEvent struct {
	Seq        int64       `json:"seq"`
	BlockID    utils.ID    `json:"block_id"`
	FromStatus Status      `json:"from_status"`
	ToStatus   Status      `json:"to_status"`
	OccurredAt time.Time   `json:"occurred_at"`
	Reason     EventReason `json:"reason"`
}
