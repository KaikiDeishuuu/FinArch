package model

import "time"

// AccountType distinguishes fund ownership.
type AccountType string

const (
	// AccountTypePersonal is the user's own funds (personal advance, etc.)
	AccountTypePersonal AccountType = "personal"
	// AccountTypePublic is company-controlled funds managed by the user.
	AccountTypePublic AccountType = "public"
)

// Account holds a cash pool with an auto-maintained cached balance.
type Account struct {
	ID           string
	UserID       string
	Name         string
	Type         AccountType
	Currency     string
	BalanceCents int64
	// Version is incremented by the balance trigger on every write — used for optimistic locking.
	Version   int64
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// BalanceYuan returns the cached balance expressed in yuan.
func (a Account) BalanceYuan() Money {
	return Money(float64(a.BalanceCents) / 100.0)
}
