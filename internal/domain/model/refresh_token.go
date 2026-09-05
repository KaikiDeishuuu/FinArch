package model

import "time"

// AuthSession is one revocable login family. Its absolute expiry never slides.
type AuthSession struct {
	ID                string
	UserID            string
	PwdVersion        int
	AbsoluteExpiresAt time.Time
	RevokedAt         *time.Time
	RevokeReason      string
	CreatedAt         time.Time
	LastUsedAt        time.Time
}

// RefreshToken is one generation within an AuthSession. TokenHash is the
// SHA-256 digest of the opaque token; raw bearer material must never be stored.
type RefreshToken struct {
	ID                 string
	SessionID          string
	TokenHash          []byte
	Generation         int
	ExpiresAt          time.Time
	ConsumedAt         *time.Time
	ReplacedByHash     []byte
	RetryUntil         *time.Time
	RecoveryCiphertext []byte
	CreatedAt          time.Time
}

// RefreshTokenSuccessor contains only random material for a possible next
// generation. In particular it cannot carry user, session, credential-version,
// or expiry data that could override the authoritative old session.
type RefreshTokenSuccessor struct {
	ID                 string
	TokenHash          []byte
	RecoveryCiphertext []byte
}

// SessionPrincipal is the authoritative identity derived by joining an active
// refresh generation to its session and current user row.
type SessionPrincipal struct {
	SessionID         string
	UserID            string
	Email             string
	Username          string
	Nickname          string
	Role              string
	PwdVersion        int
	AbsoluteExpiresAt time.Time
	RefreshExpiresAt  time.Time
	RefreshGeneration int
}

// RefreshTokenRotation is returned after a committed rotation or an idempotent
// grace-window retry. RecoveryCiphertext is authenticated encrypted bearer
// material, never a token or token hash.
type RefreshTokenRotation struct {
	Principal          SessionPrincipal
	SuccessorHash      []byte
	RecoveryCiphertext []byte
}
