package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
	"finarch/internal/infrastructure/auth"

	"github.com/google/uuid"
)

const (
	DefaultRefreshTokenTTL        = 7 * 24 * time.Hour
	DefaultSessionAbsoluteTTL     = 30 * 24 * time.Hour
	DefaultRefreshRetryGrace      = 10 * time.Second
	DefaultSessionCleanupInterval = time.Minute
	refreshTokenEntropyBytes      = 32
	refreshRecoveryKeyDomain      = "finarch/refresh-recovery/aes-256-gcm/v1"
	refreshRecoveryAADDomain      = "finarch/refresh-successor/v1\x00"
)

type SessionServiceConfig struct {
	RefreshTTL  time.Duration
	AbsoluteTTL time.Duration
	RetryGrace  time.Duration
	Now         func() time.Time
	Random      io.Reader
}

// SessionTokens contains bearer material returned only to the authentication
// boundary. RefreshToken is explicitly excluded from JSON serialization.
type SessionTokens struct {
	AccessToken       string    `json:"token"`
	AccessExpiresAt   time.Time `json:"expires_at"`
	RefreshToken      string    `json:"-"`
	RefreshExpiresAt  time.Time `json:"-"`
	AbsoluteExpiresAt time.Time `json:"-"`
	SessionID         string    `json:"-"`
	UserID            string    `json:"user_id"`
	Email             string    `json:"email"`
	Username          string    `json:"username"`
	Nickname          string    `json:"nickname"`
	Role              string    `json:"role"`
}

// SessionService owns refresh bearer generation/recovery and delegates all
// authoritative state transitions to RefreshTokenRepository.
type SessionService struct {
	repository  repository.RefreshTokenRepository
	jwt         *auth.JWTService
	aead        cipher.AEAD
	refreshTTL  time.Duration
	absoluteTTL time.Duration
	retryGrace  time.Duration
	now         func() time.Time
	random      io.Reader
}

func NewSessionService(refreshRepository repository.RefreshTokenRepository, jwt *auth.JWTService, baseSecret string, configs ...SessionServiceConfig) (*SessionService, error) {
	if refreshRepository == nil || jwt == nil || baseSecret == "" {
		return nil, fmt.Errorf("session service requires repository, JWT service, and secret")
	}
	config := SessionServiceConfig{
		RefreshTTL: DefaultRefreshTokenTTL, AbsoluteTTL: DefaultSessionAbsoluteTTL,
		RetryGrace: DefaultRefreshRetryGrace, Now: time.Now, Random: rand.Reader,
	}
	if len(configs) > 0 {
		provided := configs[0]
		if provided.RefreshTTL > 0 {
			config.RefreshTTL = provided.RefreshTTL
		}
		if provided.AbsoluteTTL > 0 {
			config.AbsoluteTTL = provided.AbsoluteTTL
		}
		if provided.RetryGrace > 0 {
			config.RetryGrace = provided.RetryGrace
		}
		if provided.Now != nil {
			config.Now = provided.Now
		}
		if provided.Random != nil {
			config.Random = provided.Random
		}
	}
	if config.RefreshTTL <= 0 || config.AbsoluteTTL <= 0 || config.RetryGrace <= 0 || config.RetryGrace >= config.RefreshTTL {
		return nil, fmt.Errorf("session durations must be positive and retry grace must be shorter than refresh TTL")
	}

	mac := hmac.New(sha256.New, []byte(baseSecret))
	_, _ = mac.Write([]byte(refreshRecoveryKeyDomain))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, fmt.Errorf("create refresh recovery cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create refresh recovery AEAD: %w", err)
	}
	return &SessionService{
		repository: refreshRepository, jwt: jwt, aead: aead,
		refreshTTL: config.RefreshTTL, absoluteTTL: config.AbsoluteTTL,
		retryGrace: config.RetryGrace, now: config.Now, random: config.Random,
	}, nil
}

func (s *SessionService) CreateSession(ctx context.Context, user model.User) (SessionTokens, error) {
	now := s.now().UTC().Truncate(time.Second)
	absoluteExpiresAt := now.Add(s.absoluteTTL)
	refreshExpiresAt := now.Add(s.refreshTTL)
	if refreshExpiresAt.After(absoluteExpiresAt) {
		refreshExpiresAt = absoluteExpiresAt
	}
	rawRefresh, refreshHash, err := s.newRefreshToken()
	if err != nil {
		return SessionTokens{}, err
	}
	sessionID := uuid.NewString()
	if err := s.repository.CreateSession(ctx, model.AuthSession{
		ID: sessionID, UserID: user.ID, PwdVersion: user.PwdVersion,
		AbsoluteExpiresAt: absoluteExpiresAt, CreatedAt: now, LastUsedAt: now,
	}, model.RefreshToken{
		ID: uuid.NewString(), SessionID: sessionID, TokenHash: refreshHash,
		Generation: 0, ExpiresAt: refreshExpiresAt, CreatedAt: now,
	}); err != nil {
		if errors.Is(err, repository.ErrSessionInvalid) {
			return SessionTokens{}, ErrSessionInvalid
		}
		return SessionTokens{}, err
	}
	accessToken, accessExpiresAt, err := s.jwt.Issue(user.ID, user.Email, user.Role, user.PwdVersion, sessionID)
	if err != nil {
		_ = s.repository.RevokeByTokenHash(ctx, refreshHash, now, "access_issue_failed")
		return SessionTokens{}, err
	}
	return SessionTokens{
		AccessToken: accessToken, AccessExpiresAt: accessExpiresAt,
		RefreshToken: rawRefresh, RefreshExpiresAt: refreshExpiresAt,
		AbsoluteExpiresAt: absoluteExpiresAt, SessionID: sessionID,
		UserID: user.ID, Email: user.Email, Username: user.Username,
		Nickname: user.Nickname, Role: user.Role,
	}, nil
}

func (s *SessionService) RotateSession(ctx context.Context, rawRefresh string) (SessionTokens, error) {
	if !isCanonicalRawRefreshToken(rawRefresh) {
		return SessionTokens{}, ErrInvalidOrUsedToken
	}
	oldHash := hashRefreshToken(rawRefresh)
	rawSuccessor, successorHash, err := s.newRefreshToken()
	if err != nil {
		return SessionTokens{}, err
	}
	recoveryCiphertext, err := s.encryptSuccessor(rawSuccessor, successorHash)
	if err != nil {
		return SessionTokens{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	rotation, err := s.repository.Rotate(ctx, oldHash, model.RefreshTokenSuccessor{
		ID: uuid.NewString(), TokenHash: successorHash, RecoveryCiphertext: recoveryCiphertext,
	}, now, s.refreshTTL, s.retryGrace)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrRefreshTokenReuse):
			return SessionTokens{}, ErrRefreshTokenReuse
		case errors.Is(err, repository.ErrRefreshTokenInvalid), errors.Is(err, repository.ErrSessionInvalid):
			return SessionTokens{}, ErrInvalidOrUsedToken
		default:
			return SessionTokens{}, err
		}
	}
	recoveredRefresh, err := s.decryptSuccessor(rotation.RecoveryCiphertext, rotation.SuccessorHash)
	if err != nil {
		return SessionTokens{}, fmt.Errorf("recover refresh successor: %w", err)
	}
	principal := rotation.Principal
	accessToken, accessExpiresAt, err := s.jwt.Issue(
		principal.UserID, principal.Email, principal.Role, principal.PwdVersion, principal.SessionID,
	)
	if err != nil {
		return SessionTokens{}, err
	}
	return SessionTokens{
		AccessToken: accessToken, AccessExpiresAt: accessExpiresAt,
		RefreshToken: recoveredRefresh, RefreshExpiresAt: principal.RefreshExpiresAt,
		AbsoluteExpiresAt: principal.AbsoluteExpiresAt, SessionID: principal.SessionID,
		UserID: principal.UserID, Email: principal.Email, Username: principal.Username,
		Nickname: principal.Nickname, Role: principal.Role,
	}, nil
}

func (s *SessionService) RevokeSession(ctx context.Context, rawRefresh string) error {
	if !isCanonicalRawRefreshToken(rawRefresh) {
		return nil
	}
	return s.repository.RevokeByTokenHash(ctx, hashRefreshToken(rawRefresh), s.now().UTC(), "logout")
}

func isCanonicalRawRefreshToken(raw string) bool {
	if len(raw) != base64.RawURLEncoding.EncodedLen(refreshTokenEntropyBytes) {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || len(decoded) != refreshTokenEntropyBytes {
		return false
	}
	return base64.RawURLEncoding.EncodeToString(decoded) == raw
}

func (s *SessionService) RevokeAllForUser(ctx context.Context, userID string) error {
	return s.repository.RevokeAllForUser(ctx, userID, s.now().UTC())
}

func (s *SessionService) ValidateAccessSession(ctx context.Context, claims *auth.Claims) error {
	if claims == nil || claims.SessionID == "" || claims.UserID == "" {
		return ErrSessionInvalid
	}
	if err := s.repository.ValidateSession(ctx, claims.SessionID, claims.UserID, claims.PwdVersion, s.now().UTC()); err != nil {
		if errors.Is(err, repository.ErrSessionInvalid) {
			return ErrSessionInvalid
		}
		return err
	}
	return nil
}

// AuthenticateAccess verifies a signed access bearer and then checks the
// authoritative session family. JWT validation errors remain distinguishable
// from session-store failures so the HTTP layer can avoid turning outages into
// misleading authentication failures.
func (s *SessionService) AuthenticateAccess(ctx context.Context, rawAccess string) (*auth.Claims, error) {
	claims, err := s.jwt.Verify(rawAccess)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateAccessSession(ctx, claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func (s *SessionService) DeleteExpired(ctx context.Context) error {
	return s.repository.DeleteExpired(ctx, s.now().UTC())
}

// RunCleanupWorker removes expired session families and short-lived refresh
// recovery material once at startup and then periodically until ctx is
// cancelled. The ticker is always stopped before the worker returns.
func (s *SessionService) RunCleanupWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultSessionCleanupInterval
	}
	cleanup := func() {
		if err := s.DeleteExpired(ctx); err != nil && ctx.Err() == nil {
			log.Printf("[auth] session cleanup failed: %v", err)
		}
	}
	if ctx.Err() != nil {
		return
	}
	cleanup()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}

func (s *SessionService) newRefreshToken() (string, []byte, error) {
	value := make([]byte, refreshTokenEntropyBytes)
	if _, err := io.ReadFull(s.random, value); err != nil {
		return "", nil, fmt.Errorf("generate refresh token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(value)
	return raw, hashRefreshToken(raw), nil
}

func hashRefreshToken(raw string) []byte {
	digest := sha256.Sum256([]byte(raw))
	return append([]byte(nil), digest[:]...)
}

func (s *SessionService) encryptSuccessor(raw string, tokenHash []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(s.random, nonce); err != nil {
		return nil, fmt.Errorf("generate refresh recovery nonce: %w", err)
	}
	aad := append([]byte(refreshRecoveryAADDomain), tokenHash...)
	return s.aead.Seal(nonce, nonce, []byte(raw), aad), nil
}

func (s *SessionService) decryptSuccessor(ciphertext, tokenHash []byte) (string, error) {
	if len(ciphertext) < s.aead.NonceSize() || len(tokenHash) != sha256.Size {
		return "", errors.New("invalid refresh recovery material")
	}
	nonce := ciphertext[:s.aead.NonceSize()]
	aad := append([]byte(refreshRecoveryAADDomain), tokenHash...)
	plaintext, err := s.aead.Open(nil, nonce, ciphertext[s.aead.NonceSize():], aad)
	if err != nil {
		return "", errors.New("invalid refresh recovery material")
	}
	return string(plaintext), nil
}
