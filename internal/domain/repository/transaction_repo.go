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
	// ListByUser returns all transactions for a given user ordered by occurred_at desc.
	ListByUser(ctx context.Context, userID string, mode model.Mode) ([]model.Transaction, error)
	// ListUnreimbursedPersonalExpenses lists unreimbursed personal expenses for a user.
	ListUnreimbursedPersonalExpenses(ctx context.Context, userID string, projectID *string, maxN int, mode model.Mode) ([]model.Transaction, error)
	// MarkReimbursed marks transactions as reimbursed and binds reimbursement ID.
	MarkReimbursed(ctx context.Context, transactionIDs []string, reimbursementID string) error
	// ToggleReimbursed flips the reimbursed flag for a single transaction owned by userID and returns the new state.
	ToggleReimbursed(ctx context.Context, id string, userID string) (bool, error)
	// ToggleUploaded flips the uploaded flag for a single transaction owned by userID and returns the new state.
	ToggleUploaded(ctx context.Context, id string, userID string) (bool, error)
	// SumPoolBalance returns company balance and personal outstanding in yuan for a user.
	SumPoolBalance(ctx context.Context, userID string, mode model.Mode) (model.Money, model.Money, error)
	// HasUnreimbursedByAccount returns true when the account has expense transactions
	// with reimb_status='pending'. Used to guard sub-account deletion.
	HasUnreimbursedByAccount(ctx context.Context, accountID, userID string) (bool, error)
}
