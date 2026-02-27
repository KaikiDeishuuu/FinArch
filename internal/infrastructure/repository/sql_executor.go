package repository

import (
	"context"
	"database/sql"
)

type contextKey string

const txContextKey contextKey = "tx"

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getExecutor(ctx context.Context, db *sql.DB) sqlExecutor {
	if tx, ok := ctx.Value(txContextKey).(*sql.Tx); ok {
		return tx
	}
	return db
}
