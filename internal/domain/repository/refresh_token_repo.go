package repository

import (
	"context"
	"time"

	"finarch/internal/domain/model"
)

// RefreshTokenRepository provides atomic refresh token operations.
type RefreshTokenRepository interface {
	// Issue persists a new refresh token row.
	Issue(ctx context.Context, t model.RefreshToken) error
	// Rotate atomically consumes the old token (by hash) and issues the new one.
	// Returns ErrInvalidOrUsedToken when the token is missing, already consumed, or expired.
	Rotate(ctx context.Context, oldHash string, newToken model.RefreshToken) (model.RefreshToken, error)
	// RevokeAllForUser marks every active token for a user as consumed.
	RevokeAllForUser(ctx context.Context, userID string, now time.Time) error
	// DeleteExpired removes rows that were consumed or expired before cutoff.
	DeleteExpired(ctx context.Context, cutoff time.Time) error
}
