package repository

import (
	"context"
	"time"

	"finarch/internal/domain/model"
)

// UserRepository defines user data access.
type UserRepository interface {
	Create(ctx context.Context, user model.User) error
	GetByEmail(ctx context.Context, email string) (model.User, error)
	GetByID(ctx context.Context, id string) (model.User, error)
	UpdatePassword(ctx context.Context, id, passwordHash string) error
	SetEmailVerified(ctx context.Context, id string) error

	// Email token management (verification + password reset + deletion + email change)
	CreateEmailToken(ctx context.Context, t model.EmailToken) error
	GetEmailToken(ctx context.Context, token string) (model.EmailToken, error)
	DeleteEmailToken(ctx context.Context, token string) error
	DeleteEmailTokensByUser(ctx context.Context, userID, kind string) error

	// Email change flow
	SetPendingEmail(ctx context.Context, id, pendingEmail string) error
	UpdateEmail(ctx context.Context, id, newEmail string) error // also clears pending_email

	// UpdateNickname updates the user's display nickname.
	UpdateNickname(ctx context.Context, id, nickname string) error

	// DeleteUser permanently removes the user and all their data.
	DeleteUser(ctx context.Context, id string) error

	// One-time action request management for signed token flows.
	CreateActionRequest(ctx context.Context, req model.ActionRequest) error
	GetActionRequestByJTI(ctx context.Context, jti string) (model.ActionRequest, error)
	ConsumeActionRequest(ctx context.Context, jti string, consumedAt time.Time) (model.ActionRequest, error)
	ExpireActionRequests(ctx context.Context, action string, now time.Time) error

	// Security/audit trail.
	CreateAuditEvent(ctx context.Context, userID, eventType, ipAddr, deviceMeta string) error

	// DeleteExpiredUnverifiedUsers removes unverified users whose created_at < olderThan,
	// along with their tokens. Returns the number of deleted users.
	DeleteExpiredUnverifiedUsers(ctx context.Context, olderThan time.Time) (int64, error)
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
