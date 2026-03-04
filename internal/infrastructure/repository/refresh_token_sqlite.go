package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
)

// SQLiteRefreshTokenRepository stores refresh tokens in SQLite.
type SQLiteRefreshTokenRepository struct {
	db *sql.DB
}

// NewSQLiteRefreshTokenRepository creates a new refresh token repository.
func NewSQLiteRefreshTokenRepository(db *sql.DB) *SQLiteRefreshTokenRepository {
	return &SQLiteRefreshTokenRepository{db: db}
}

// Issue inserts a new refresh token row.
func (r *SQLiteRefreshTokenRepository) Issue(ctx context.Context, t model.RefreshToken) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at, version)
		 VALUES (?, ?, ?, ?, ?, 1)`,
		t.ID, t.UserID, t.TokenHash,
		t.ExpiresAt.UTC().Format(time.RFC3339Nano),
		t.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("issue refresh token: %w", err)
	}
	return nil
}

// Rotate atomically consumes the old token and issues the new one within a transaction.
// Exactly ONE concurrent caller will succeed; all others get ErrInvalidOrUsedToken.
func (r *SQLiteRefreshTokenRepository) Rotate(ctx context.Context, oldHash string, newToken model.RefreshToken) (model.RefreshToken, error) {
	tm := NewSQLiteTransactionManager(r.db)
	var result model.RefreshToken
	err := tm.WithinTransaction(ctx, func(txCtx context.Context) error {
		exec := getExecutor(txCtx, r.db)
		now := time.Now().UTC().Format(time.RFC3339Nano)

		// Step 1: atomically consume the old token (WHERE consumed_at IS NULL AND expires_at > NOW)
		res, err := exec.ExecContext(txCtx,
			`UPDATE refresh_tokens
			 SET consumed_at = ?, version = version + 1
			 WHERE token_hash = ?
			   AND consumed_at IS NULL
			   AND expires_at > ?`,
			now, oldHash, now,
		)
		if err != nil {
			return fmt.Errorf("consume refresh token: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return service.ErrInvalidOrUsedToken
		}

		// Step 2: issue the new rotated token.
		_, err = exec.ExecContext(txCtx,
			`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at, version)
			 VALUES (?, ?, ?, ?, ?, 1)`,
			newToken.ID, newToken.UserID, newToken.TokenHash,
			newToken.ExpiresAt.UTC().Format(time.RFC3339Nano),
			newToken.CreatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			return fmt.Errorf("issue rotated refresh token: %w", err)
		}
		result = newToken
		return nil
	})
	return result, err
}

// RevokeAllForUser marks every active token for the user as consumed.
func (r *SQLiteRefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string, now time.Time) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE refresh_tokens
		 SET consumed_at = ?, version = version + 1
		 WHERE user_id = ? AND consumed_at IS NULL`,
		now.UTC().Format(time.RFC3339Nano), userID,
	)
	return err
}

// DeleteExpired removes tokens that were consumed or expired before the cutoff.
func (r *SQLiteRefreshTokenRepository) DeleteExpired(ctx context.Context, cutoff time.Time) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`DELETE FROM refresh_tokens
		 WHERE consumed_at IS NOT NULL OR expires_at < ?`,
		cutoff.UTC().Format(time.RFC3339Nano),
	)
	return err
}
