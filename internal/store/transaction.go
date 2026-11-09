package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// transact releases the transaction on every path, including a failed commit.
func (s *Store) transact(ctx context.Context, work func(*sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin planner save: %w", classify(err))
	}
	defer func() {
		if cleanup := tx.Rollback(); cleanup != nil && !errors.Is(cleanup, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback planner save: %w", cleanup))
		}
	}()
	if err := work(tx); err != nil {
		return classify(err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit planner save: %w", classify(err))
	}
	return nil
}
