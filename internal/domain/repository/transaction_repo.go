package repository

import (
	"context"

	"finarch/internal/domain/model"
)

// TransactionRepository defines transaction data access behavior.
type TransactionRepository interface {
	// Create inserts one transaction.
	Create(ctx context.Context, transaction model.Transaction) error
	// GetByIDs loads transactions by IDs.
	GetByIDs(ctx context.Context, ids []string) ([]model.Transaction, error)
	// ListAll returns all transactions ordered by occurred_at desc.
	ListAll(ctx context.Context) ([]model.Transaction, error)
	// ListUnreimbursedPersonalExpenses lists unreimbursed personal expenses.
	ListUnreimbursedPersonalExpenses(ctx context.Context, projectID *string, maxN int) ([]model.Transaction, error)
	// MarkReimbursed marks transactions as reimbursed and binds reimbursement ID.
	MarkReimbursed(ctx context.Context, transactionIDs []string, reimbursementID string) error
	// ToggleReimbursed flips the reimbursed flag for a single transaction and returns the new state.
	ToggleReimbursed(ctx context.Context, id string) (bool, error)
	// ToggleUploaded flips the uploaded flag for a single transaction and returns the new state.
	ToggleUploaded(ctx context.Context, id string) (bool, error)
	// SumPoolBalance returns company balance and personal outstanding in yuan.
	SumPoolBalance(ctx context.Context) (model.Money, model.Money, error)
}
