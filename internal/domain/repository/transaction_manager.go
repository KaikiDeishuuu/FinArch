package repository

import "context"

// TransactionManager executes function within one database transaction.
type TransactionManager interface {
	// WithinTransaction runs fn in one transaction and auto commits or rollbacks.
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
