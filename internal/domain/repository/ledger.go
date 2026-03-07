package repository

import (
	"context"

	"finarch/internal/domain/model"
)

// LedgerRepository defines persistence operations for the financial ledger engine.
type LedgerRepository interface {
	// CreateAccount inserts one ledger account.
	CreateAccount(ctx context.Context, a model.LedgerAccount) error
	// GetAccount returns a ledger account by ID.
	GetAccount(ctx context.Context, id string) (model.LedgerAccount, error)

	// CreateJournalEntry persists one entry and its lines atomically, updating balance cache
	// and appending an audit event. Implementations must enforce sum(debit)=sum(credit).
	CreateJournalEntry(ctx context.Context, entry model.LedgerJournalEntry, lines []model.LedgerJournalLine, eventType string, payloadJSON string) error

	// GetAccountBalance returns the cached balance for an account, rebuilding from lines if missing.
	GetAccountBalance(ctx context.Context, userID, accountID string) (model.LedgerBalance, error)

	// ValidateIntegrity runs consistency checks (double-entry, orphan lines, balance vs lines).
	ValidateIntegrity(ctx context.Context, userID string) error

	// RebuildBalanceCache recomputes balances from journal lines.
	RebuildBalanceCache(ctx context.Context, userID string) error
}

