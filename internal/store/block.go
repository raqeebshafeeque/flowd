package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/utils"
)

var ErrBlockNotFound = errors.New("block not found")

// BlockByID reads a saved block, including an archived block.
func (s *Store) BlockByID(ctx context.Context, id utils.ID) (*domain.Block, error) {
	block, err := scanBlock(s.db.QueryRowContext(ctx, "SELECT "+blockColumns+" FROM blocks WHERE id = ?", id))
	if err != nil {
		return nil, fmt.Errorf("find block by id %q: %w", id, err)
	}
	return block, nil
}

const blockColumns = `id, day_id, title, status, archived, planned_start_utc,
	planned_end_utc, actual_start_utc, actual_end_utc, copied_from_id, created_at_utc, updated_at_utc`

// BlocksByDay lists visible blocks by actual planned instant, not timestamp text.
func (s *Store) BlocksByDay(ctx context.Context, id utils.ID) ([]domain.Block, error) {
	if _, err := s.DayByID(ctx, id); err != nil {
		return nil, err
	}
	blocks, err := readBlocks(ctx, s.db, id)
	if err != nil {
		return nil, fmt.Errorf("list blocks for day %q: %w", id, err)
	}
	return blocks, nil
}

type blockReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// Used both by collection reads and final-plan checks inside write transactions.
func readBlocks(ctx context.Context, reader blockReader, dayID utils.ID) ([]domain.Block, error) {
	rows, err := reader.QueryContext(ctx, "SELECT "+blockColumns+" FROM blocks WHERE day_id = ? AND archived = 0", dayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	blocks := make([]domain.Block, 0)
	for rows.Next() {
		block, err := scanBlock(rows)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, *block)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].PlannedStart.Equal(blocks[j].PlannedStart) {
			return blocks[i].ID < blocks[j].ID
		}
		return blocks[i].PlannedStart.Before(blocks[j].PlannedStart)
	})
	return blocks, nil
}

func insertBlock(ctx context.Context, tx *sql.Tx, block domain.Block) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO blocks ("+blockColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", blockValues(block)...)
	if err != nil {
		return fmt.Errorf("insert block %q: %w", block.ID, err)
	}
	return nil
}

func blockValues(block domain.Block) []any {
	var source any
	if block.CopiedFromID != nil {
		source = string(*block.CopiedFromID)
	}
	return []any{block.ID, block.DayID, block.Title, block.Status, block.Archived,
		block.PlannedStart.UTC().Format(time.RFC3339Nano), block.PlannedEnd.UTC().Format(time.RFC3339Nano),
		nullableTime(block.ActualStart), nullableTime(block.ActualEnd), source,
		block.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(block.UpdatedAt)}
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

// Both one-row and collection reads share the same saved-field mapping.
func scanBlock(row interface{ Scan(...any) error }) (*domain.Block, error) {
	var block domain.Block
	var start, end, created string
	var actualStart, actualEnd, updated, source sql.NullString
	if err := row.Scan(&block.ID, &block.DayID, &block.Title, &block.Status, &block.Archived,
		&start, &end, &actualStart, &actualEnd, &source, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBlockNotFound
		}
		return nil, err
	}
	for _, field := range []struct {
		name, value string
		target      *time.Time
	}{
		{"planned start", start, &block.PlannedStart}, {"planned end", end, &block.PlannedEnd}, {"creation time", created, &block.CreatedAt},
	} {
		stamp, err := time.Parse(time.RFC3339Nano, field.value)
		if err != nil {
			return nil, fmt.Errorf("parse block %q %s: %w", block.ID, field.name, err)
		}
		*field.target = stamp.UTC()
	}
	for _, field := range []struct {
		name   string
		value  sql.NullString
		target **time.Time
	}{
		{"actual start", actualStart, &block.ActualStart}, {"actual end", actualEnd, &block.ActualEnd}, {"update time", updated, &block.UpdatedAt},
	} {
		if !field.value.Valid {
			continue
		}
		stamp, err := time.Parse(time.RFC3339Nano, field.value.String)
		if err != nil {
			return nil, fmt.Errorf("parse block %q %s: %w", block.ID, field.name, err)
		}
		stamp = stamp.UTC()
		*field.target = &stamp
	}
	if source.Valid {
		id := utils.ID(source.String)
		block.CopiedFromID = &id
	}
	return &block, nil
}
