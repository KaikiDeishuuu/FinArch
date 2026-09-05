package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mattn/go-sqlite3"
)

const (
	transactionBeginRetryLimit = 5 * time.Second
	transactionBeginRetryDelay = 2 * time.Millisecond
)

// SQLiteTransactionManager provides transaction behavior for SQLite repositories.
type SQLiteTransactionManager struct {
	db *sql.DB
}

// NewSQLiteTransactionManager creates a new SQLiteTransactionManager.
func NewSQLiteTransactionManager(db *sql.DB) *SQLiteTransactionManager {
	return &SQLiteTransactionManager{db: db}
}

// WithinTransaction runs fn in one transaction and auto commits or rollbacks.
func (m *SQLiteTransactionManager) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := beginSQLiteTransaction(ctx, m.db)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	txCtx := context.WithValue(ctx, txContextKey, tx)
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// BEGIN IMMEDIATE normally honors SQLite's busy timeout, but shared-cache
// contention reports SQLITE_LOCKED immediately instead. Retrying the begin
// keeps a concurrent domain operation behind the winning writer so it can
// observe the committed state and return its semantic conflict outcome.
func beginSQLiteTransaction(ctx context.Context, database *sql.DB) (*sql.Tx, error) {
	deadline := time.Now().Add(transactionBeginRetryLimit)
	for {
		tx, err := database.BeginTx(ctx, nil)
		if err == nil {
			return tx, nil
		}
		if !isSQLiteContention(err) || !time.Now().Before(deadline) {
			return nil, err
		}

		timer := time.NewTimer(transactionBeginRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func isSQLiteContention(err error) bool {
	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) &&
		(sqliteErr.Code == sqlite3.ErrBusy || sqliteErr.Code == sqlite3.ErrLocked)
}
