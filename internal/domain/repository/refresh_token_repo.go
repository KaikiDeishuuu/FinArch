package repository

import (
	"context"
	"time"

	"finarch/internal/domain/model"
)

// RefreshTokenRepository provides atomic refresh token operations.
type RefreshTokenRepository interface {
	// CreateSession atomically persists a new session and its generation zero.
	CreateSession(ctx context.Context, session model.AuthSession, token model.RefreshToken) error
	// Rotate atomically consumes oldHash and creates exactly one successor. All
	// identity and expiry authority is derived from the old row and session.
	Rotate(ctx context.Context, oldHash []byte, successor model.RefreshTokenSuccessor, now time.Time, refreshTTL, retryGrace time.Duration) (model.RefreshTokenRotation, error)
	// RevokeByTokenHash idempotently revokes the family containing tokenHash.
	RevokeByTokenHash(ctx context.Context, tokenHash []byte, now time.Time, reason string) error
	// RevokeAllForUser revokes every active session for a user.
	RevokeAllForUser(ctx context.Context, userID string, now time.Time) error
	// ValidateSession verifies access-token session and credential binding.
	ValidateSession(ctx context.Context, sessionID, userID string, pwdVersion int, now time.Time) error
	// DeleteExpired clears expired recovery material and deletes families only
	// after their absolute expiry, retaining consumed generations for replay proof.
	DeleteExpired(ctx context.Context, now time.Time) error
}
