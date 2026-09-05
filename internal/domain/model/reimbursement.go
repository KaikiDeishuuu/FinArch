package model

import "time"

// Reimbursement is one reimbursement request.
type Reimbursement struct {
	ID        string
	RequestNo string
	Applicant string
	// TotalCents is the canonical CNY amount. TotalYuan is retained as a
	// backward-compatible presentation value only.
	TotalCents int64
	TotalYuan  Money
	Status     string
	PaidAt     *time.Time
	Version    int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ReimbursementItem links reimbursement and transaction.
type ReimbursementItem struct {
	ReimbursementID string
	TransactionID   string
	// AmountCents is the canonical CNY amount. AmountYuan is legacy display
	// data and must not be used for financial aggregation.
	AmountCents int64
	AmountYuan  Money
}
