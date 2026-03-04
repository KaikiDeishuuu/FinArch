package model

import "time"

// User represents an authenticated system user.
type User struct {
	ID            string
	Email         string
	Username      string // unique, immutable after registration
	Nickname      string // mutable display name; shown in greetings etc.
	Name          string // kept for backward compat; mirrors Username
	PendingEmail  string // unverified new email during email-change flow
	PasswordHash  string
	PwdVersion    int
	Role          string // reserved; all users are equal
	EmailVerified bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// EmailToken is a short-lived token for email verification, password reset,
// account deletion, or email change.
type EmailToken struct {
	Token     string
	UserID    string
	Kind      string // "verify" | "reset" | "delete" | "change_email"
	Meta      string // extra payload (e.g. new email for change_email)
	ExpiresAt time.Time
	CreatedAt time.Time
}

// AccountDeletionRequest tracks one-time account deletion confirmations.
type AccountDeletionRequest struct {
	JTI       string
	UserID    string
	Status    string // pending | completed | expired
	ExpiresAt time.Time
	UsedAt    *time.Time
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
