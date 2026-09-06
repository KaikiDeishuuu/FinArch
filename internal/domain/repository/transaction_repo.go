package repository

import (
	"context"

	"finarch/internal/domain/model"
)

// TransactionRepository defines transaction data access behavior.
type TransactionRepository interface {
	// Create inserts one transaction.
	Create(ctx context.Context, transaction model.Transaction) error
	// GetByIDs loads transactions by IDs owned by userID.
	GetByIDs(ctx context.Context, userID string, ids []string) ([]model.Transaction, error)
	// GetByIDForUser loads one transaction by ID and owner.
	GetByIDForUser(ctx context.Context, id, userID string) (model.Transaction, error)
	// GetByIdempotencyKey loads one transaction for an idempotency key.
	GetByIdempotencyKey(ctx context.Context, userID, key string) (model.Transaction, error)
	// ClaimIdempotencyKey atomically reserves a user/endpoint-scoped key. When
	// claimed is false, storedRequestHash belongs to the existing reservation.
	ClaimIdempotencyKey(ctx context.Context, userID, endpoint, key, requestHash string) (storedRequestHash string, claimed bool, err error)
	// SetAttachmentKey links the primary attachment key to a transaction.
	SetAttachmentKey(ctx context.Context, transactionID, userID, key string) error
	// ClearAttachmentKey removes the primary attachment key when it still matches the deleted attachment.
	ClearAttachmentKey(ctx context.Context, transactionID, userID, key string) error
	// ListByUser returns all transactions for a given user ordered by occurred_at desc.
	ListByUser(ctx context.Context, userID string, mode model.Mode) ([]model.Transaction, error)
	// ListUnreimbursedPersonalExpenses lists unreimbursed personal expenses for a user.
	ListUnreimbursedPersonalExpenses(ctx context.Context, userID string, projectID *string, maxN int, mode model.Mode) ([]model.Transaction, error)
	// MarkReimbursed marks transactions owned by userID as reimbursed and binds reimbursement ID.
	MarkReimbursed(ctx context.Context, userID string, transactionIDs []string, reimbursementID string) error
	// ToggleReimbursed flips the reimbursed flag for a single transaction owned by userID and returns the new state.
	ToggleReimbursed(ctx context.Context, id string, userID string) (bool, error)
	// ToggleUploaded flips the uploaded flag for a single transaction owned by userID and returns the new state.
	ToggleUploaded(ctx context.Context, id string, userID string) (bool, error)
	// ToggleSettled flips the settled flag for a single public-account WORK expense owned by userID and returns the new state.
	ToggleSettled(ctx context.Context, id string, userID string) (bool, error)
	// SumPoolBalance returns company balance and personal outstanding in yuan for a user.
	SumPoolBalance(ctx context.Context, userID string, mode model.Mode) (model.Money, model.Money, error)
	// HasTransactionsByAccount returns true when the account has any historical
	// transaction. Accounts with history must be archived/transferred rather
	// than deleted so their balances and statements remain visible.
	HasTransactionsByAccount(ctx context.Context, accountID, userID string) (bool, error)
	// GetRecentRate returns the most recently persisted rate for a currency pair and base currency.
	GetRecentRate(ctx context.Context, userID, fromCurrency, baseCurrency string) (rate float64, rateAt int64, source string, err error)
}
