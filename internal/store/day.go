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

var ErrDayNotFound = errors.New("day not found")

const dayColumns = "id, date, timezone, created_at_utc, updated_at_utc"

// DayByDate finds a saved day or returns ErrDayNotFound.
func (s *Store) DayByDate(ctx context.Context, date string) (*domain.Day, error) {
	day, err := scanDay(s.db.QueryRowContext(ctx, "SELECT "+dayColumns+" FROM days WHERE date = ?", date))
	if err != nil {
		return nil, fmt.Errorf("find day by date %q: %w", date, err)
	}
	return day, nil
}

// DayByID includes the original saved identity and timestamps.
func (s *Store) DayByID(ctx context.Context, id utils.ID) (*domain.Day, error) {
	day, err := scanDay(s.db.QueryRowContext(ctx, "SELECT "+dayColumns+" FROM days WHERE id = ?", id))
	if err != nil {
		return nil, fmt.Errorf("find day by id %q: %w", id, err)
	}
	return day, nil
}

func scanDay(row *sql.Row) (*domain.Day, error) {
	var day domain.Day
	var created string
	var updated sql.NullString
	err := row.Scan(&day.ID, &day.Date, &day.Timezone, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDayNotFound
	}
	if err != nil {
		return nil, err
	}
	day.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return nil, fmt.Errorf("parse creation time: %w", err)
	}
	day.CreatedAt = day.CreatedAt.UTC()
	if updated.Valid {
		stamp, err := time.Parse(time.RFC3339Nano, updated.String)
		if err != nil {
			return nil, fmt.Errorf("parse update time: %w", err)
		}
		stamp = stamp.UTC()
		day.UpdatedAt = &stamp
	}
	return &day, nil
}

// CreateDay inserts a day without replacing an existing identity or date.
func (s *Store) CreateDay(ctx context.Context, day domain.Day) error {
	var updated any
	if day.UpdatedAt != nil {
		updated = day.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO days
		(id, date, timezone, created_at_utc, updated_at_utc) VALUES (?, ?, ?, ?, ?)`,
		day.ID, day.Date, day.Timezone, day.CreatedAt.UTC().Format(time.RFC3339Nano), updated)
	if err != nil {
		return fmt.Errorf("create day %q: %w", day.ID, err)
	}
	return nil
}
