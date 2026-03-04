package model

import "time"

// RefreshToken holds a rotatable refresh token stored in the database.
// On each use, the current token is atomically consumed and a new one is issued.
type RefreshToken struct {
	ID         string
	UserID     string
	TokenHash  string // bcrypt or SHA-256 hash of the raw token
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
	Version    int
}
