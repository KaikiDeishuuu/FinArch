package repository

import (
	"context"

	"finarch/internal/domain/model"
)

// AccountRepository defines account persistence behavior.
type AccountRepository interface {
	// Create inserts one account.
	Create(ctx context.Context, a model.Account) error
	// GetByID loads an account by its primary key.
	GetByID(ctx context.Context, id string) (model.Account, error)
	// ListByUser returns all active accounts for a user.
	ListByUser(ctx context.Context, userID string) ([]model.Account, error)
	// GetByUserAndType returns the account of a given type for a user (creates on demand when needed).
	GetByUserAndType(ctx context.Context, userID string, t model.AccountType) (model.Account, error)
	// Update saves name and is_active changes to an account.
	Update(ctx context.Context, a model.Account) error
	// UpdateName saves only the name of an account atomically.
	UpdateName(ctx context.Context, id, userID, newName string) error
	// Delete removes an account by ID (only if it belongs to the user).
	Delete(ctx context.Context, id, userID string) error
	// CountByUserAndType returns how many active accounts of the given type a user has.
	CountByUserAndType(ctx context.Context, userID string, t model.AccountType) (int, error)
}
