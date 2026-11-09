package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/utils"
)

var (
	ErrInvalidEvent = errors.New("event does not match block change")
	ErrStaleBlock   = errors.New("block status changed since it was loaded")
)

func validateEvent(block domain.Block, event domain.BlockEvent) error {
	if event.BlockID != block.ID || event.ToStatus != block.Status || event.Seq != 0 || event.OccurredAt.IsZero() {
		return fmt.Errorf("block %q: %w", block.ID, ErrInvalidEvent)
	}
	return nil
}

// CreateBlock inserts a block and its initial event on an existing day.
func (s *Store) CreateBlock(ctx context.Context, block domain.Block, event domain.BlockEvent) error {
	return s.createBlock(ctx, nil, block, event)
}

// CreateDayWithBlock atomically creates a day, its first block and the event.
func (s *Store) CreateDayWithBlock(ctx context.Context, day domain.Day, block domain.Block, event domain.BlockEvent) error {
	return s.createBlock(ctx, &day, block, event)
}

func (s *Store) createBlock(ctx context.Context, day *domain.Day, block domain.Block, event domain.BlockEvent) error {
	if err := validateEvent(block, event); err != nil {
		return err
	}
	if event.FromStatus != block.Status || (day != nil && day.ID != block.DayID) {
		return ErrInvalidEvent
	}
	return s.transact(ctx, func(tx *sql.Tx) error {
		if day != nil {
			if err := insertDay(ctx, tx, *day); err != nil {
				return err
			}
		}
		if err := insertBlock(ctx, tx, block); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, event); err != nil {
			return err
		}
		return checkPlan(ctx, tx, block.DayID)
	})
}

// BlockChange pairs one resulting block with the event explaining its mutation.
type BlockChange struct {
	Block domain.Block
	Event domain.BlockEvent
}

// SaveMoves saves all supplied changes together, including automatic pauses.
func (s *Store) SaveMoves(ctx context.Context, changes []BlockChange) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		seen := make(map[utils.ID]bool)
		days := make(map[utils.ID]bool)
		for _, change := range changes {
			block := change.Block
			if seen[block.ID] {
				return fmt.Errorf("duplicate block id %q: %w", block.ID, ErrInvalidEvent)
			}
			seen[block.ID] = true
			if err := validateEvent(block, change.Event); err != nil {
				return err
			}
			previous, err := scanBlock(tx.QueryRowContext(ctx, "SELECT "+blockColumns+" FROM blocks WHERE id=?", block.ID))
			if err != nil {
				return err
			}
			if previous.Status != change.Event.FromStatus {
				return fmt.Errorf("block %q: %w", block.ID, ErrStaleBlock)
			}
			sameSource := previous.CopiedFromID == nil && block.CopiedFromID == nil
			if previous.CopiedFromID != nil && block.CopiedFromID != nil {
				sameSource = *previous.CopiedFromID == *block.CopiedFromID
			}
			if previous.Archived || previous.DayID != block.DayID || !previous.CreatedAt.Equal(block.CreatedAt) || !sameSource {
				return fmt.Errorf("immutable or archived block %q: %w", block.ID, ErrInvalidEvent)
			}
			stamp := change.Event.OccurredAt.UTC()
			block.UpdatedAt = &stamp
			args := append(blockValues(block)[1:], block.ID)
			result, err := tx.ExecContext(ctx, `UPDATE blocks SET day_id=?, title=?, status=?, archived=?,
				planned_start_utc=?, planned_end_utc=?, actual_start_utc=?, actual_end_utc=?,
				copied_from_id=?, created_at_utc=?, updated_at_utc=? WHERE id=?`, args...)
			if err != nil {
				return fmt.Errorf("save block %q: %w", block.ID, err)
			}
			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if count == 0 {
				return ErrBlockNotFound
			}
			if err := insertEvent(ctx, tx, change.Event); err != nil {
				return err
			}
			days[block.DayID] = true
		}
		for id := range days {
			if err := checkPlan(ctx, tx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// ArchiveBlock keeps status and history; a repeated archive is a no-op.
func (s *Store) ArchiveBlock(ctx context.Context, id utils.ID, stamp time.Time) error {
	block, err := s.BlockByID(ctx, id)
	if err != nil {
		return err
	}
	if block.Archived {
		return nil
	}
	block.Archived = true
	return s.SaveMoves(ctx, []BlockChange{{Block: *block, Event: domain.BlockEvent{
		BlockID: id, FromStatus: block.Status, ToStatus: block.Status, OccurredAt: stamp, Reason: domain.ReasonUser,
	}}})
}

// EventsByBlock includes archived history, ordered by sequence rather than time.
func (s *Store) EventsByBlock(ctx context.Context, id utils.ID) ([]domain.BlockEvent, error) {
	if _, err := s.BlockByID(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT seq,block_id,from_status,to_status,occurred_at_utc,reason
		FROM block_events WHERE block_id=? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]domain.BlockEvent, 0)
	for rows.Next() {
		var event domain.BlockEvent
		var stamp string
		if err := rows.Scan(&event.Seq, &event.BlockID, &event.FromStatus, &event.ToStatus, &stamp, &event.Reason); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			return nil, fmt.Errorf("parse event time: %w", err)
		}
		event.OccurredAt = parsed.UTC()
		events = append(events, event)
	}
	return events, rows.Err()
}

func checkPlan(ctx context.Context, tx *sql.Tx, dayID utils.ID) error {
	blocks, err := readBlocks(ctx, tx, dayID)
	if err != nil {
		return err
	}
	if err := domain.ValidatePlan(blocks); err != nil {
		return fmt.Errorf("validate day %q: %w", dayID, err)
	}
	return nil
}

func insertEvent(ctx context.Context, tx *sql.Tx, event domain.BlockEvent) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO block_events
  (block_id,from_status,to_status,occurred_at_utc,reason) VALUES (?, ?, ?, ?, ?)`,
		event.BlockID, event.FromStatus, event.ToStatus, event.OccurredAt.UTC().Format(time.RFC3339Nano), event.Reason)
	if err != nil {
		return fmt.Errorf("insert event for block %q: %w", event.BlockID, err)
	}
	return nil
}
