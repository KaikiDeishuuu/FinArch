package repository

import (
	"context"
	"database/sql"
	"fmt"
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
	tx, err := m.db.BeginTx(ctx, nil)
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
