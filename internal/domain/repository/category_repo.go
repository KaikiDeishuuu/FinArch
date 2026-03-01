package repository

import (
	"context"

	"finarch/internal/domain/model"
)

// CategoryRepository defines category persistence behavior.
type CategoryRepository interface {
	// Create inserts one category.
	Create(ctx context.Context, c model.Category) error
	// ListByUser returns all active categories for a user.
	ListByUser(ctx context.Context, userID string) ([]model.Category, error)
	// GetByID loads a category by ID.
	GetByID(ctx context.Context, id string) (model.Category, error)
	// Update saves name, sort_order and is_active changes.
	Update(ctx context.Context, c model.Category) error
	// Delete hard-deletes a category (only safe when no transactions reference it).
	Delete(ctx context.Context, id string, userID string) error
}
