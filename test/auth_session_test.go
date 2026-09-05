package test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/service"
	"finarch/internal/infrastructure/auth"
	sqliterepo "finarch/internal/infrastructure/repository"
)

type mutableSessionClock struct {
	mu  sync.RWMutex
	now time.Time
}

func (c *mutableSessionClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *mutableSessionClock) Set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

func configuredSessionService(
	t *testing.T,
	database *sql.DB,
	jwt *auth.JWTService,
	clock *mutableSessionClock,
	refreshTTL, absoluteTTL, retryGrace time.Duration,
) *service.SessionService {
	t.Helper()
	sessions, err := service.NewSessionService(
		sqliterepo.NewSQLiteRefreshTokenRepository(database),
		jwt,
		"test-secret",
		service.SessionServiceConfig{
			RefreshTTL:  refreshTTL,
			AbsoluteTTL: absoluteTTL,
			RetryGrace:  retryGrace,
			Now:         clock.Now,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return sessions
}

func sessionTestUser(t *testing.T, database *sql.DB) model.User {
	t.Helper()
	user, err := sqliterepo.NewSQLiteUserRepository(database).GetByID(context.Background(), testUserID)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func TestRefreshBearerIsOpaqueHashedAndJSONExcluded(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	clock := &mutableSessionClock{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	jwt := auth.NewJWTService("test-secret")
	sessions := configuredSessionService(t, database, jwt, clock, time.Hour, 24*time.Hour, 5*time.Second)

	tokens, err := sessions.CreateSession(context.Background(), sessionTestUser(t, database))
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens.RefreshToken) != 43 {
		t.Fatalf("opaque refresh length = %d, want 43 base64url characters", len(tokens.RefreshToken))
	}

	encoded, err := json.Marshal(tokens)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(tokens.RefreshToken)) || bytes.Contains(encoded, []byte("refresh_token")) {
		t.Fatalf("refresh bearer leaked through JSON: %s", encoded)
	}

	var storageType string
	var storedHash []byte
	if err := database.QueryRow(`SELECT typeof(token_hash), token_hash FROM refresh_tokens`).Scan(&storageType, &storedHash); err != nil {
		t.Fatal(err)
	}
	if storageType != "blob" || len(storedHash) != sha256.Size {
		t.Fatalf("stored token = type %q length %d, want 32-byte BLOB", storageType, len(storedHash))
	}
	wantHash := sha256.Sum256([]byte(tokens.RefreshToken))
	if !bytes.Equal(storedHash, wantHash[:]) {
		t.Fatal("database does not contain the SHA-256 digest of the opaque bearer")
	}
	if bytes.Equal(storedHash, []byte(tokens.RefreshToken)) {
		t.Fatal("database stored the raw refresh bearer")
	}

	claims, err := jwt.Verify(tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.SessionID == "" || claims.SessionID != tokens.SessionID {
		t.Fatalf("access sid = %q, want %q", claims.SessionID, tokens.SessionID)
	}
}

func TestRefreshRotationGraceChainAndReplayRevocation(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	clock := &mutableSessionClock{now: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)}
	jwt := auth.NewJWTService("test-secret")
	sessions := configuredSessionService(t, database, jwt, clock, time.Hour, 24*time.Hour, 10*time.Second)

	initial, err := sessions.CreateSession(ctx, sessionTestUser(t, database))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE users
		SET email = 'current@example.com', username = 'current-user', nickname = 'Current', role = 'owner'
		WHERE id = ?`, testUserID); err != nil {
		t.Fatal(err)
	}

	generationOne, err := sessions.RotateSession(ctx, initial.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if generationOne.SessionID != initial.SessionID || generationOne.UserID != initial.UserID {
		t.Fatal("rotation changed the authoritative session identity")
	}
	if generationOne.Email != "current@example.com" || generationOne.Username != "current-user" || generationOne.Role != "owner" {
		t.Fatalf("rotation did not derive current identity from the user row: %+v", generationOne)
	}

	retryOne, err := sessions.RotateSession(ctx, initial.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if retryOne.RefreshToken != generationOne.RefreshToken {
		t.Fatal("lost-response retry returned a different successor")
	}

	generationTwo, err := sessions.RotateSession(ctx, generationOne.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	chainedRetry, err := sessions.RotateSession(ctx, initial.RefreshToken)
	if err != nil {
		t.Fatalf("ancestor retry inside grace revoked a valid chain: %v", err)
	}
	if chainedRetry.RefreshToken != generationTwo.RefreshToken {
		t.Fatal("ancestor retry did not recover the latest active successor")
	}
	var generationCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE session_id = ?`, initial.SessionID).Scan(&generationCount); err != nil {
		t.Fatal(err)
	}
	if generationCount != 3 {
		t.Fatalf("refresh generation count = %d, want 3", generationCount)
	}
	claims, err := jwt.Verify(generationTwo.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.ValidateAccessSession(ctx, claims); err != nil {
		t.Fatalf("valid chain unexpectedly revoked: %v", err)
	}

	clock.Set(clock.Now().Add(11 * time.Second))
	if _, err := sessions.RotateSession(ctx, initial.RefreshToken); !errors.Is(err, service.ErrRefreshTokenReuse) {
		t.Fatalf("ancestor replay outside grace = %v, want refresh reuse", err)
	}
	var revokedAt sql.NullInt64
	var reason sql.NullString
	if err := database.QueryRowContext(ctx, `SELECT revoked_at, revoke_reason FROM auth_sessions WHERE id = ?`, initial.SessionID).Scan(&revokedAt, &reason); err != nil {
		t.Fatal(err)
	}
	if !revokedAt.Valid || reason.String != "refresh_reuse" {
		t.Fatalf("replay revocation not committed: revoked=%v reason=%q", revokedAt.Valid, reason.String)
	}
	if _, err := sessions.RotateSession(ctx, generationTwo.RefreshToken); !errors.Is(err, service.ErrInvalidOrUsedToken) {
		t.Fatalf("latest child after family revocation = %v, want invalid token", err)
	}
	if err := sessions.ValidateAccessSession(ctx, claims); !errors.Is(err, service.ErrSessionInvalid) {
		t.Fatalf("access token after family revocation = %v, want invalid session", err)
	}
}

func TestRefreshRetryNearExpiryRecoversUnexpiredTerminalSuccessor(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	startedAt := time.Date(2026, 2, 4, 5, 6, 7, 0, time.UTC)
	clock := &mutableSessionClock{now: startedAt}
	jwt := auth.NewJWTService("test-secret")
	sessions := configuredSessionService(t, database, jwt, clock, 10*time.Second, time.Hour, 5*time.Second)

	initial, err := sessions.CreateSession(ctx, sessionTestUser(t, database))
	if err != nil {
		t.Fatal(err)
	}

	// Rotate one second before the original generation expires, then retry it
	// after expiry while its lost-response retry window remains open.
	clock.Set(startedAt.Add(9 * time.Second))
	generationOne, err := sessions.RotateSession(ctx, initial.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	clock.Set(startedAt.Add(11 * time.Second))
	recovered, err := sessions.RotateSession(ctx, initial.RefreshToken)
	if err != nil {
		t.Fatalf("near-expiry ancestor retry inside grace: %v", err)
	}
	if recovered.RefreshToken != generationOne.RefreshToken {
		t.Fatal("near-expiry ancestor retry did not recover the active terminal successor")
	}
	var revokedAt sql.NullInt64
	if err := database.QueryRowContext(ctx, `SELECT revoked_at FROM auth_sessions WHERE id = ?`, initial.SessionID).Scan(&revokedAt); err != nil {
		t.Fatal(err)
	}
	if revokedAt.Valid {
		t.Fatal("near-expiry retry revoked a session with an active successor")
	}

	clock.Set(startedAt.Add(15 * time.Second))
	if _, err := sessions.RotateSession(ctx, initial.RefreshToken); !errors.Is(err, service.ErrRefreshTokenReuse) {
		t.Fatalf("expired ancestor outside grace = %v, want refresh reuse", err)
	}
	var reason sql.NullString
	if err := database.QueryRowContext(ctx, `SELECT revoked_at, revoke_reason FROM auth_sessions WHERE id = ?`, initial.SessionID).Scan(&revokedAt, &reason); err != nil {
		t.Fatal(err)
	}
	if !revokedAt.Valid || reason.String != "refresh_reuse" {
		t.Fatalf("outside-grace replay not revoked: revoked=%v reason=%q", revokedAt.Valid, reason.String)
	}
}

func TestRefreshAbsoluteExpiryCleanupAndWorkerLifecycle(t *testing.T) {
	database := setupDB(t)
	defer database.Close()
	ctx := context.Background()
	startedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	clock := &mutableSessionClock{now: startedAt}
	jwt := auth.NewJWTService("test-secret")
	sessions := configuredSessionService(t, database, jwt, clock, 10*time.Minute, 12*time.Minute, 5*time.Second)

	initial, err := sessions.CreateSession(ctx, sessionTestUser(t, database))
	if err != nil {
		t.Fatal(err)
	}
	absoluteExpiry := startedAt.Add(12 * time.Minute)
	if !initial.AbsoluteExpiresAt.Equal(absoluteExpiry) {
		t.Fatalf("absolute expiry = %v, want %v", initial.AbsoluteExpiresAt, absoluteExpiry)
	}

	clock.Set(startedAt.Add(8 * time.Minute))
	rotated, err := sessions.RotateSession(ctx, initial.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if !rotated.AbsoluteExpiresAt.Equal(absoluteExpiry) || !rotated.RefreshExpiresAt.Equal(absoluteExpiry) {
		t.Fatalf("rotation slid or exceeded absolute expiry: absolute=%v refresh=%v", rotated.AbsoluteExpiresAt, rotated.RefreshExpiresAt)
	}

	clock.Set(startedAt.Add(8*time.Minute + 5*time.Second))
	if err := sessions.DeleteExpired(ctx); err != nil {
		t.Fatal(err)
	}
	var recoveryIsNull int
	if err := database.QueryRowContext(ctx, `
		SELECT recovery_ciphertext IS NULL
		FROM refresh_tokens
		WHERE session_id = ? AND generation = 0`, initial.SessionID).Scan(&recoveryIsNull); err != nil {
		t.Fatal(err)
	}
	if recoveryIsNull != 0 {
		t.Fatal("recovery material was cleared at the inclusive grace boundary")
	}

	clock.Set(startedAt.Add(8*time.Minute + 6*time.Second))
	if err := sessions.DeleteExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `
		SELECT recovery_ciphertext IS NULL
		FROM refresh_tokens
		WHERE session_id = ? AND generation = 0`, initial.SessionID).Scan(&recoveryIsNull); err != nil {
		t.Fatal(err)
	}
	if recoveryIsNull != 1 {
		t.Fatal("recovery material remained after the retry grace window")
	}
	var tokenCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE session_id = ?`, initial.SessionID).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if tokenCount != 2 {
		t.Fatalf("consumed replay evidence deleted before family absolute expiry: count=%d", tokenCount)
	}

	clock.Set(absoluteExpiry)
	workerCtx, cancel := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		sessions.RunCleanupWorker(workerCtx, time.Hour)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		var sessionCount int
		if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM auth_sessions WHERE id = ?`, initial.SessionID).Scan(&sessionCount); err != nil {
			t.Fatal(err)
		}
		if sessionCount == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cleanup worker did not run its startup pass")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("cleanup worker did not stop after cancellation")
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE session_id = ?`, initial.SessionID).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if tokenCount != 0 {
		t.Fatalf("expired family did not cascade-delete generations: count=%d", tokenCount)
	}
}

func TestRefreshLogoutAndAccountStateInvalidation(t *testing.T) {
	t.Run("logout accepts current consumed and empty bearer", func(t *testing.T) {
		database := setupDB(t)
		defer database.Close()
		ctx := context.Background()
		clock := &mutableSessionClock{now: time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)}
		jwt := auth.NewJWTService("test-secret")
		sessions := configuredSessionService(t, database, jwt, clock, time.Hour, 24*time.Hour, 5*time.Second)

		initial, err := sessions.CreateSession(ctx, sessionTestUser(t, database))
		if err != nil {
			t.Fatal(err)
		}
		child, err := sessions.RotateSession(ctx, initial.RefreshToken)
		if err != nil {
			t.Fatal(err)
		}
		if err := sessions.RevokeSession(ctx, initial.RefreshToken); err != nil {
			t.Fatal(err)
		}
		if err := sessions.RevokeSession(ctx, initial.RefreshToken); err != nil {
			t.Fatalf("repeated logout was not idempotent: %v", err)
		}
		if err := sessions.RevokeSession(ctx, ""); err != nil {
			t.Fatalf("empty logout was not idempotent: %v", err)
		}
		if _, err := sessions.RotateSession(ctx, child.RefreshToken); !errors.Is(err, service.ErrInvalidOrUsedToken) {
			t.Fatalf("consumed-generation logout did not revoke its family: %v", err)
		}

		current, err := sessions.CreateSession(ctx, sessionTestUser(t, database))
		if err != nil {
			t.Fatal(err)
		}
		if err := sessions.RevokeSession(ctx, current.RefreshToken); err != nil {
			t.Fatal(err)
		}
		if _, err := sessions.RotateSession(ctx, current.RefreshToken); !errors.Is(err, service.ErrInvalidOrUsedToken) {
			t.Fatalf("current-generation logout did not revoke its family: %v", err)
		}
	})

	for _, mutation := range []struct {
		name string
		sql  string
	}{
		{name: "password version", sql: `UPDATE users SET pwd_version = pwd_version + 1 WHERE id = ?`},
		{name: "unverified user", sql: `UPDATE users SET email_verified = 0 WHERE id = ?`},
		{name: "soft-deleted user", sql: `UPDATE users SET deleted_at = strftime('%s','now') WHERE id = ?`},
	} {
		mutation := mutation
		t.Run(mutation.name, func(t *testing.T) {
			database := setupDB(t)
			defer database.Close()
			ctx := context.Background()
			clock := &mutableSessionClock{now: time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)}
			jwt := auth.NewJWTService("test-secret")
			sessions := configuredSessionService(t, database, jwt, clock, time.Hour, 24*time.Hour, 5*time.Second)
			tokens, err := sessions.CreateSession(ctx, sessionTestUser(t, database))
			if err != nil {
				t.Fatal(err)
			}
			claims, err := jwt.Verify(tokens.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.ExecContext(ctx, mutation.sql, testUserID); err != nil {
				t.Fatal(err)
			}
			if _, err := sessions.RotateSession(ctx, tokens.RefreshToken); !errors.Is(err, service.ErrInvalidOrUsedToken) {
				t.Fatalf("refresh after %s = %v, want invalid", mutation.name, err)
			}
			if err := sessions.ValidateAccessSession(ctx, claims); !errors.Is(err, service.ErrSessionInvalid) {
				t.Fatalf("access after %s = %v, want invalid session", mutation.name, err)
			}
		})
	}
}

func installSessionRevokeFailureTrigger(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec(`
		CREATE TRIGGER fail_auth_session_revoke
		BEFORE UPDATE OF revoked_at ON auth_sessions
		WHEN NEW.revoked_at IS NOT NULL
		BEGIN
			SELECT RAISE(ABORT, 'forced session revoke failure');
		END`); err != nil {
		t.Fatal(err)
	}
}

func assertAuthAccessSessionValid(t *testing.T, svc *service.AuthService, accessToken string) {
	t.Helper()
	claims, err := auth.NewJWTService("test-secret").Verify(accessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ValidateAccessSession(context.Background(), claims); err != nil {
		t.Fatalf("access session should remain valid after rollback: %v", err)
	}
}

func TestCredentialChangesAndSessionRevocationAreAtomic(t *testing.T) {
	t.Run("password change rollback", func(t *testing.T) {
		svc, repo, _, database, user := setupFlowAuth(t)
		ctx := context.Background()
		if _, err := database.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, user.ID); err != nil {
			t.Fatal(err)
		}
		login, err := svc.Login(ctx, user.Email, "Password123")
		if err != nil {
			t.Fatal(err)
		}
		before, err := repo.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		installSessionRevokeFailureTrigger(t, database)

		if err := svc.ChangePassword(ctx, user.ID, "Password123", "ReplacementPassword123"); err == nil {
			t.Fatal("password change succeeded despite session-revoke failure")
		}
		after, err := repo.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after.PasswordHash != before.PasswordHash || after.PwdVersion != before.PwdVersion {
			t.Fatal("password mutation committed without its session revocation")
		}
		if err := auth.CheckPassword(after.PasswordHash, "Password123"); err != nil {
			t.Fatal("old password no longer works after transaction rollback")
		}
		assertAuthAccessSessionValid(t, svc, login.Token)
	})

	t.Run("password reset and action consumption rollback", func(t *testing.T) {
		svc, repo, sender, database, user := setupFlowAuth(t)
		ctx := context.Background()
		if _, err := database.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, user.ID); err != nil {
			t.Fatal(err)
		}
		login, err := svc.Login(ctx, user.Email, "Password123")
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.ForgotPassword(ctx, user.Email); err != nil {
			t.Fatal(err)
		}
		claims, err := auth.NewActionTokenService("test-secret").Verify(sender.resetToken, service.ActionPasswordReset)
		if err != nil {
			t.Fatal(err)
		}
		before, err := repo.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		installSessionRevokeFailureTrigger(t, database)

		if err := svc.ResetPassword(ctx, sender.resetToken, "ReplacementPassword123"); err == nil {
			t.Fatal("password reset succeeded despite session-revoke failure")
		}
		after, err := repo.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after.PasswordHash != before.PasswordHash || after.PwdVersion != before.PwdVersion {
			t.Fatal("password reset committed without its session revocation")
		}
		var actionStatus string
		if err := database.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE jti = ?`, claims.ID).Scan(&actionStatus); err != nil {
			t.Fatal(err)
		}
		if actionStatus != "pending" {
			t.Fatalf("reset action was consumed despite rollback: status=%s", actionStatus)
		}
		assertAuthAccessSessionValid(t, svc, login.Token)
	})

	t.Run("email change and action consumption rollback", func(t *testing.T) {
		svc, repo, sender, database, user := setupFlowAuth(t)
		ctx := context.Background()
		if _, err := database.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, user.ID); err != nil {
			t.Fatal(err)
		}
		login, err := svc.Login(ctx, user.Email, "Password123")
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.RequestEmailChange(ctx, user.ID, "Password123", "replacement@example.com"); err != nil {
			t.Fatal(err)
		}
		if err := svc.ConfirmOldEmailForChange(ctx, sender.emailOldToken); err != nil {
			t.Fatal(err)
		}
		claims, err := auth.NewActionTokenService("test-secret").Verify(sender.emailNewToken, service.ActionEmailChangeNew)
		if err != nil {
			t.Fatal(err)
		}
		installSessionRevokeFailureTrigger(t, database)

		if err := svc.ConfirmEmailChange(ctx, sender.emailNewToken); err == nil {
			t.Fatal("email change succeeded despite session-revoke failure")
		}
		after, err := repo.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after.Email != user.Email {
			t.Fatalf("email committed without its session revocation: %s", after.Email)
		}
		var actionStatus string
		if err := database.QueryRowContext(ctx, `SELECT status FROM action_requests WHERE jti = ?`, claims.ID).Scan(&actionStatus); err != nil {
			t.Fatal(err)
		}
		if actionStatus != "pending" {
			t.Fatalf("email action was consumed despite rollback: status=%s", actionStatus)
		}
		assertAuthAccessSessionValid(t, svc, login.Token)
	})
}

func TestAccountDeletionInvalidatesRefreshAndAccessSessions(t *testing.T) {
	svc, _, sender, database, user := setupFlowAuth(t)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, user.ID); err != nil {
		t.Fatal(err)
	}
	login, err := svc.Login(ctx, user.Email, "Password123")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := auth.NewJWTService("test-secret").Verify(login.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RequestAccountDeletion(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmAccountDeletion(ctx, sender.deleteToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RefreshSession(ctx, login.RefreshToken); !errors.Is(err, service.ErrInvalidOrUsedToken) {
		t.Fatalf("refresh after account deletion = %v, want invalid", err)
	}
	if err := svc.ValidateAccessSession(ctx, claims); !errors.Is(err, service.ErrSessionInvalid) {
		t.Fatalf("access after account deletion = %v, want invalid session", err)
	}
	var sessionsRemaining int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM auth_sessions WHERE user_id = ?`, user.ID).Scan(&sessionsRemaining); err != nil {
		t.Fatal(err)
	}
	if sessionsRemaining != 0 {
		t.Fatalf("account deletion left %d auth sessions", sessionsRemaining)
	}
}

func TestPasswordChangeRacingRefreshLeavesNoLiveSession(t *testing.T) {
	svc, _, _, database, user := setupFlowAuth(t)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, user.ID); err != nil {
		t.Fatal(err)
	}
	login, err := svc.Login(ctx, user.Email, "Password123")
	if err != nil {
		t.Fatal(err)
	}
	initialClaims, err := auth.NewJWTService("test-secret").Verify(login.Token)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	passwordResult := make(chan error, 1)
	refreshResult := make(chan struct {
		response service.LoginResponse
		err      error
	}, 1)
	go func() {
		<-start
		passwordResult <- svc.ChangePassword(ctx, user.ID, "Password123", "ReplacementPassword123")
	}()
	go func() {
		<-start
		response, err := svc.RefreshSession(ctx, login.RefreshToken)
		refreshResult <- struct {
			response service.LoginResponse
			err      error
		}{response: response, err: err}
	}()
	close(start)
	if err := <-passwordResult; err != nil {
		t.Fatalf("password change lost refresh race: %v", err)
	}
	refresh := <-refreshResult

	if err := svc.ValidateAccessSession(ctx, initialClaims); !errors.Is(err, service.ErrSessionInvalid) {
		t.Fatalf("initial access survived password change: %v", err)
	}
	if refresh.err == nil {
		claims, err := auth.NewJWTService("test-secret").Verify(refresh.response.Token)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.ValidateAccessSession(ctx, claims); !errors.Is(err, service.ErrSessionInvalid) {
			t.Fatalf("racing refresh access survived password change: %v", err)
		}
		if _, err := svc.RefreshSession(ctx, refresh.response.RefreshToken); !errors.Is(err, service.ErrInvalidOrUsedToken) {
			t.Fatalf("racing refresh successor survived password change: %v", err)
		}
	} else if !errors.Is(refresh.err, service.ErrInvalidOrUsedToken) {
		t.Fatalf("unexpected refresh race error: %v", refresh.err)
	}
}

func TestRefreshSessionMigrationHasOnlyHashedBearerStorage(t *testing.T) {
	database := setupDB(t)
	defer database.Close()

	rows, err := database.Query(`PRAGMA table_info(refresh_tokens)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := make(map[string]string)
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = columnType
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if columns["token_hash"] != "BLOB" {
		t.Fatalf("token_hash column type = %q, want BLOB", columns["token_hash"])
	}
	for _, forbidden := range []string{"token", "refresh_token", "raw_token"} {
		if _, exists := columns[forbidden]; exists {
			t.Fatalf("refresh_tokens migration retained plaintext bearer column %q", forbidden)
		}
	}

	assertCascade := func(table, referencedTable string) {
		t.Helper()
		fkRows, err := database.Query(`PRAGMA foreign_key_list(` + table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		defer fkRows.Close()
		found := false
		for fkRows.Next() {
			var id, seq int
			var target, from, to, onUpdate, onDelete, match string
			if err := fkRows.Scan(&id, &seq, &target, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				t.Fatal(err)
			}
			if target == referencedTable && onDelete == "CASCADE" {
				found = true
			}
		}
		if err := fkRows.Err(); err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("%s does not cascade-delete with %s", table, referencedTable)
		}
	}
	assertCascade("refresh_tokens", "auth_sessions")
	assertCascade("auth_sessions", "users")
}
