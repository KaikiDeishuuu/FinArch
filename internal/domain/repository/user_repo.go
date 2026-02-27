package repository

import (
	"context"

	"finarch/internal/domain/model"
)

// UserRepository defines user data access.
type UserRepository interface {
	Create(ctx context.Context, user model.User) error
	GetByEmail(ctx context.Context, email string) (model.User, error)
	GetByID(ctx context.Context, id string) (model.User, error)
}

// TagRepository defines tag data access.
type TagRepository interface {
	Create(ctx context.Context, tag model.Tag) error
	ListByOwner(ctx context.Context, ownerID string) ([]model.Tag, error)
	Delete(ctx context.Context, id, ownerID string) error
	AddToTransaction(ctx context.Context, transactionID, tagID string) error
	RemoveFromTransaction(ctx context.Context, transactionID, tagID string) error
	ListByTransaction(ctx context.Context, transactionID string) ([]model.Tag, error)
}
