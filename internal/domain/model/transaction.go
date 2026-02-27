package model

import "time"

// Direction indicates income or expense.
type Direction string

const (
	// DirectionIncome is incoming cash.
	DirectionIncome Direction = "income"
	// DirectionExpense is outgoing cash.
	DirectionExpense Direction = "expense"
)

// Source indicates where money comes from.
type Source string

const (
	// SourceCompany means company funds.
	SourceCompany Source = "company"
	// SourcePersonal means personal advance/payment.
	SourcePersonal Source = "personal"
)

// Transaction records one cash movement.
type Transaction struct {
	ID              string
	OccurredAt      time.Time
	Direction       Direction
	Source          Source
	Category        string
	AmountYuan      Money
	Currency        string
	Note            string
	ProjectID       *string
	Reimbursed      bool
	Uploaded        bool
	ReimbursementID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
