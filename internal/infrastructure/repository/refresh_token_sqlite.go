package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"finarch/internal/domain/model"
	domainrepo "finarch/internal/domain/repository"
)

// SQLiteRefreshTokenRepository stores revocable login families and hashed
// refresh-token generations. Raw bearer tokens never cross this boundary.
type SQLiteRefreshTokenRepository struct {
	db         *sql.DB
	rotationMu sync.Mutex
}

func NewSQLiteRefreshTokenRepository(db *sql.DB) *SQLiteRefreshTokenRepository {
	return &SQLiteRefreshTokenRepository{db: db}
}

func (r *SQLiteRefreshTokenRepository) CreateSession(ctx context.Context, session model.AuthSession, token model.RefreshToken) error {
	if session.ID == "" || session.UserID == "" || token.ID == "" || token.SessionID != session.ID ||
		token.Generation != 0 || len(token.TokenHash) != 32 || !session.AbsoluteExpiresAt.After(session.CreatedAt) ||
		!token.ExpiresAt.After(token.CreatedAt) || token.ExpiresAt.After(session.AbsoluteExpiresAt) {
		return domainrepo.ErrSessionInvalid
	}

	tm := NewSQLiteTransactionManager(r.db)
	return tm.WithinTransaction(ctx, func(txCtx context.Context) error {
		exec := getExecutor(txCtx, r.db)
		res, err := exec.ExecContext(txCtx, `
			INSERT INTO auth_sessions (
				id, user_id, pwd_version, absolute_expires_at, revoked_at,
				revoke_reason, created_at, last_used_at
			)
			SELECT ?, u.id, u.pwd_version, ?, NULL, NULL, ?, ?
			FROM users u
			WHERE u.id = ?
			  AND u.deleted_at IS NULL
			  AND u.email_verified = 1
			  AND COALESCE(u.pwd_version, 0) = ?`,
			session.ID, session.AbsoluteExpiresAt.UTC().Unix(), session.CreatedAt.UTC().Unix(),
			session.LastUsedAt.UTC().Unix(), session.UserID, session.PwdVersion)
		if err != nil {
			return fmt.Errorf("create auth session: %w", err)
		}
		if affected, _ := res.RowsAffected(); affected != 1 {
			return domainrepo.ErrSessionInvalid
		}
		if _, err := exec.ExecContext(txCtx, `
			INSERT INTO refresh_tokens (
				id, session_id, token_hash, generation, expires_at, consumed_at,
				replaced_by_hash, retry_until, recovery_ciphertext, created_at
			) VALUES (?, ?, ?, 0, ?, NULL, NULL, NULL, NULL, ?)`,
			token.ID, session.ID, token.TokenHash, token.ExpiresAt.UTC().Unix(), token.CreatedAt.UTC().Unix()); err != nil {
			return fmt.Errorf("create refresh generation: %w", err)
		}
		return nil
	})
}

type refreshRotationState struct {
	tokenID            string
	sessionID          string
	generation         int
	tokenExpiresAt     int64
	consumedAt         sql.NullInt64
	replacedByHash     []byte
	retryUntil         sql.NullInt64
	recoveryCiphertext []byte
	userID             string
	sessionPwdVersion  int
	absoluteExpiresAt  int64
	revokedAt          sql.NullInt64
	email              string
	username           string
	nickname           string
	role               string
	emailVerified      int
	currentPwdVersion  int
	userDeletedAt      sql.NullInt64
}

// Rotate serializes on an IMMEDIATE SQLite transaction (configured by the
// application's DSN). It returns errors only after replay revocation commits.
func (r *SQLiteRefreshTokenRepository) Rotate(ctx context.Context, oldHash []byte, successor model.RefreshTokenSuccessor, now time.Time, refreshTTL, retryGrace time.Duration) (model.RefreshTokenRotation, error) {
	if len(oldHash) != 32 || successor.ID == "" || len(successor.TokenHash) != 32 || len(successor.RecoveryCiphertext) == 0 || refreshTTL <= 0 || retryGrace <= 0 {
		return model.RefreshTokenRotation{}, domainrepo.ErrRefreshTokenInvalid
	}
	r.rotationMu.Lock()
	defer r.rotationMu.Unlock()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.RefreshTokenRotation{}, fmt.Errorf("begin refresh rotation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var state refreshRotationState
	err = tx.QueryRowContext(ctx, `
		SELECT rt.id, rt.session_id, rt.generation, rt.expires_at, rt.consumed_at,
		       rt.replaced_by_hash, rt.retry_until, rt.recovery_ciphertext,
		       s.user_id, s.pwd_version, s.absolute_expires_at, s.revoked_at,
		       u.email, COALESCE(u.username, u.name), COALESCE(u.nickname, ''), u.role,
		       u.email_verified, COALESCE(u.pwd_version, 0), u.deleted_at
		FROM refresh_tokens rt
		JOIN auth_sessions s ON s.id = rt.session_id
		JOIN users u ON u.id = s.user_id
		WHERE rt.token_hash = ?`, oldHash).Scan(
		&state.tokenID, &state.sessionID, &state.generation, &state.tokenExpiresAt, &state.consumedAt,
		&state.replacedByHash, &state.retryUntil, &state.recoveryCiphertext,
		&state.userID, &state.sessionPwdVersion, &state.absoluteExpiresAt, &state.revokedAt,
		&state.email, &state.username, &state.nickname, &state.role,
		&state.emailVerified, &state.currentPwdVersion, &state.userDeletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.RefreshTokenRotation{}, domainrepo.ErrRefreshTokenInvalid
	}
	if err != nil {
		return model.RefreshTokenRotation{}, fmt.Errorf("load refresh generation: %w", err)
	}

	now = now.UTC()
	nowUnix := now.Unix()
	principal := model.SessionPrincipal{
		SessionID:         state.sessionID,
		UserID:            state.userID,
		Email:             state.email,
		Username:          state.username,
		Nickname:          state.nickname,
		Role:              state.role,
		PwdVersion:        state.sessionPwdVersion,
		AbsoluteExpiresAt: time.Unix(state.absoluteExpiresAt, 0).UTC(),
	}

	if state.revokedAt.Valid {
		return model.RefreshTokenRotation{}, domainrepo.ErrRefreshTokenInvalid
	}
	invalidReason := ""
	switch {
	case state.userDeletedAt.Valid || state.emailVerified != 1 || state.currentPwdVersion != state.sessionPwdVersion:
		invalidReason = "credentials_changed"
	case state.absoluteExpiresAt <= nowUnix:
		invalidReason = "absolute_expired"
	}
	if invalidReason != "" {
		if err := revokeSessionTx(ctx, tx, state.sessionID, nowUnix, invalidReason); err != nil {
			return model.RefreshTokenRotation{}, err
		}
		if err := tx.Commit(); err != nil {
			return model.RefreshTokenRotation{}, fmt.Errorf("commit invalid session revocation: %w", err)
		}
		return model.RefreshTokenRotation{}, domainrepo.ErrRefreshTokenInvalid
	}

	if state.consumedAt.Valid {
		if state.retryUntil.Valid && nowUnix <= state.retryUntil.Int64 && len(state.replacedByHash) == 32 && len(state.recoveryCiphertext) > 0 {
			// A delayed response can overwrite the browser cookie with an older
			// generation after another tab has already advanced the family. Follow
			// the authenticated successor chain and recover the newest active token
			// instead of returning a consumed intermediate generation.
			successorHash := append([]byte(nil), state.replacedByHash...)
			recoveryCiphertext := append([]byte(nil), state.recoveryCiphertext...)
			expectedGeneration := state.generation + 1
			const maxRecoveryHops = 256
			for hop := 0; hop < maxRecoveryHops; hop++ {
				var childGeneration int
				var childExpiresAt int64
				var childConsumedAt, childRetryUntil sql.NullInt64
				var childReplacedByHash, childRecoveryCiphertext []byte
				err := tx.QueryRowContext(ctx, `
					SELECT generation, expires_at, consumed_at, replaced_by_hash,
					       retry_until, recovery_ciphertext
					FROM refresh_tokens
					WHERE token_hash = ? AND session_id = ?`,
					successorHash, state.sessionID,
				).Scan(
					&childGeneration, &childExpiresAt, &childConsumedAt, &childReplacedByHash,
					&childRetryUntil, &childRecoveryCiphertext,
				)
				if errors.Is(err, sql.ErrNoRows) {
					break
				}
				if err != nil {
					return model.RefreshTokenRotation{}, fmt.Errorf("load refresh successor: %w", err)
				}
				if childGeneration != expectedGeneration {
					break
				}
				if !childConsumedAt.Valid {
					// Consumed ancestors and intermediate generations may naturally
					// expire while their lost-response retry windows are still open.
					// Only the terminal active generation must remain unexpired.
					if childExpiresAt <= nowUnix {
						if err := revokeSessionTx(ctx, tx, state.sessionID, nowUnix, "refresh_expired"); err != nil {
							return model.RefreshTokenRotation{}, err
						}
						if err := tx.Commit(); err != nil {
							return model.RefreshTokenRotation{}, fmt.Errorf("commit invalid session revocation: %w", err)
						}
						return model.RefreshTokenRotation{}, domainrepo.ErrRefreshTokenInvalid
					}
					if _, err := tx.ExecContext(ctx, `UPDATE auth_sessions SET last_used_at = ? WHERE id = ?`, nowUnix, state.sessionID); err != nil {
						return model.RefreshTokenRotation{}, fmt.Errorf("touch auth session: %w", err)
					}
					if err := tx.Commit(); err != nil {
						return model.RefreshTokenRotation{}, fmt.Errorf("commit refresh retry: %w", err)
					}
					principal.RefreshExpiresAt = time.Unix(childExpiresAt, 0).UTC()
					principal.RefreshGeneration = childGeneration
					return model.RefreshTokenRotation{
						Principal: principal, SuccessorHash: successorHash,
						RecoveryCiphertext: recoveryCiphertext,
					}, nil
				}
				if !childRetryUntil.Valid || nowUnix > childRetryUntil.Int64 ||
					len(childReplacedByHash) != 32 || len(childRecoveryCiphertext) == 0 {
					break
				}
				successorHash = append(successorHash[:0], childReplacedByHash...)
				recoveryCiphertext = append(recoveryCiphertext[:0], childRecoveryCiphertext...)
				expectedGeneration++
			}
			if expectedGeneration-state.generation > maxRecoveryHops {
				return model.RefreshTokenRotation{}, fmt.Errorf("refresh successor chain exceeds recovery limit")
			}
		}
		if err := revokeSessionTx(ctx, tx, state.sessionID, nowUnix, "refresh_reuse"); err != nil {
			return model.RefreshTokenRotation{}, err
		}
		if err := tx.Commit(); err != nil {
			return model.RefreshTokenRotation{}, fmt.Errorf("commit refresh replay revocation: %w", err)
		}
		return model.RefreshTokenRotation{}, domainrepo.ErrRefreshTokenReuse
	}

	if state.tokenExpiresAt <= nowUnix {
		if err := revokeSessionTx(ctx, tx, state.sessionID, nowUnix, "refresh_expired"); err != nil {
			return model.RefreshTokenRotation{}, err
		}
		if err := tx.Commit(); err != nil {
			return model.RefreshTokenRotation{}, fmt.Errorf("commit invalid session revocation: %w", err)
		}
		return model.RefreshTokenRotation{}, domainrepo.ErrRefreshTokenInvalid
	}

	refreshExpiresAt := now.Add(refreshTTL).Unix()
	if refreshExpiresAt > state.absoluteExpiresAt {
		refreshExpiresAt = state.absoluteExpiresAt
	}
	if refreshExpiresAt <= nowUnix {
		if err := revokeSessionTx(ctx, tx, state.sessionID, nowUnix, "absolute_expired"); err != nil {
			return model.RefreshTokenRotation{}, err
		}
		if err := tx.Commit(); err != nil {
			return model.RefreshTokenRotation{}, fmt.Errorf("commit expired session revocation: %w", err)
		}
		return model.RefreshTokenRotation{}, domainrepo.ErrRefreshTokenInvalid
	}

	nextGeneration := state.generation + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO refresh_tokens (
			id, session_id, token_hash, generation, expires_at, consumed_at,
			replaced_by_hash, retry_until, recovery_ciphertext, created_at
		) VALUES (?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, ?)`,
		successor.ID, state.sessionID, successor.TokenHash, nextGeneration, refreshExpiresAt, nowUnix); err != nil {
		return model.RefreshTokenRotation{}, fmt.Errorf("create refresh successor: %w", err)
	}
	retryUntil := now.Add(retryGrace).Unix()
	res, err := tx.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET consumed_at = ?, replaced_by_hash = ?, retry_until = ?, recovery_ciphertext = ?
		WHERE id = ? AND consumed_at IS NULL`,
		nowUnix, successor.TokenHash, retryUntil, successor.RecoveryCiphertext, state.tokenID)
	if err != nil {
		return model.RefreshTokenRotation{}, fmt.Errorf("consume refresh generation: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return model.RefreshTokenRotation{}, fmt.Errorf("consume refresh generation: %w", domainrepo.ErrRefreshTokenInvalid)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE auth_sessions SET last_used_at = ? WHERE id = ?`, nowUnix, state.sessionID); err != nil {
		return model.RefreshTokenRotation{}, fmt.Errorf("touch auth session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.RefreshTokenRotation{}, fmt.Errorf("commit refresh rotation: %w", err)
	}
	principal.RefreshExpiresAt = time.Unix(refreshExpiresAt, 0).UTC()
	principal.RefreshGeneration = nextGeneration
	return model.RefreshTokenRotation{
		Principal: principal, SuccessorHash: append([]byte(nil), successor.TokenHash...),
		RecoveryCiphertext: append([]byte(nil), successor.RecoveryCiphertext...),
	}, nil
}

func revokeSessionTx(ctx context.Context, tx *sql.Tx, sessionID string, nowUnix int64, reason string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE auth_sessions
		SET revoked_at = COALESCE(revoked_at, ?),
		    revoke_reason = COALESCE(revoke_reason, ?)
		WHERE id = ?`, nowUnix, reason, sessionID); err != nil {
		return fmt.Errorf("revoke auth session: %w", err)
	}
	return nil
}

func (r *SQLiteRefreshTokenRepository) RevokeByTokenHash(ctx context.Context, tokenHash []byte, now time.Time, reason string) error {
	if len(tokenHash) != 32 {
		return nil
	}
	if reason == "" {
		reason = "logout"
	}
	r.rotationMu.Lock()
	defer r.rotationMu.Unlock()
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		UPDATE auth_sessions
		SET revoked_at = COALESCE(revoked_at, ?),
		    revoke_reason = COALESCE(revoke_reason, ?)
		WHERE id = (SELECT session_id FROM refresh_tokens WHERE token_hash = ?)`,
		now.UTC().Unix(), reason, tokenHash)
	if err != nil {
		return fmt.Errorf("revoke refresh session: %w", err)
	}
	return nil
}

func (r *SQLiteRefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string, now time.Time) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		UPDATE auth_sessions
		SET revoked_at = COALESCE(revoked_at, ?),
		    revoke_reason = COALESCE(revoke_reason, 'credentials_changed')
		WHERE user_id = ?`, now.UTC().Unix(), userID)
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

func (r *SQLiteRefreshTokenRepository) ValidateSession(ctx context.Context, sessionID, userID string, pwdVersion int, now time.Time) error {
	var valid int
	err := getExecutor(ctx, r.db).QueryRowContext(ctx, `
		SELECT 1
		FROM auth_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.id = ? AND s.user_id = ?
		  AND s.pwd_version = ? AND COALESCE(u.pwd_version, 0) = s.pwd_version
		  AND s.revoked_at IS NULL AND s.absolute_expires_at > ?
		  AND u.deleted_at IS NULL AND u.email_verified = 1`,
		sessionID, userID, pwdVersion, now.UTC().Unix()).Scan(&valid)
	if errors.Is(err, sql.ErrNoRows) {
		return domainrepo.ErrSessionInvalid
	}
	if err != nil {
		return fmt.Errorf("validate auth session: %w", err)
	}
	return nil
}

func (r *SQLiteRefreshTokenRepository) DeleteExpired(ctx context.Context, now time.Time) error {
	tm := NewSQLiteTransactionManager(r.db)
	return tm.WithinTransaction(ctx, func(txCtx context.Context) error {
		exec := getExecutor(txCtx, r.db)
		nowUnix := now.UTC().Unix()
		if _, err := exec.ExecContext(txCtx, `
			UPDATE refresh_tokens
			SET recovery_ciphertext = NULL
			WHERE retry_until IS NOT NULL AND retry_until < ?`, nowUnix); err != nil {
			return fmt.Errorf("clear refresh recovery material: %w", err)
		}
		if _, err := exec.ExecContext(txCtx, `DELETE FROM auth_sessions WHERE absolute_expires_at <= ?`, nowUnix); err != nil {
			return fmt.Errorf("delete expired auth sessions: %w", err)
		}
		return nil
	})
}
