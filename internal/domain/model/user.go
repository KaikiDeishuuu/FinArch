package model

import "time"

// User represents an authenticated system user.
type User struct {
	ID            string
	Email         string
	Name          string
	PasswordHash  string
	Role          string // reserved; all users are equal
	EmailVerified bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// EmailToken is a short-lived token for email verification or password reset.
type EmailToken struct {
	Token     string
	UserID    string
	Kind      string // "verify" or "reset"
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Tag is a user-defined label that can be attached to transactions.
type Tag struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"owner_id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
}
