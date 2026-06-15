package model

import "time"

// Budget represents one active monthly spending limit for a user.
// Category is empty for a whole-month budget, otherwise it scopes the budget
// to transactions with the same category text.
type Budget struct {
	ID              string
	UserID          string
	Mode            Mode
	PeriodMonth     string // YYYY-MM
	Category        string
	AmountCents     int64
	Currency        string
	BaseCurrency    string
	BaseAmountCents int64
	IsActive        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// BudgetActuals contains expense totals for one user/mode/month window.
type BudgetActuals struct {
	TotalCents int64
	ByCategory map[string]int64
}
