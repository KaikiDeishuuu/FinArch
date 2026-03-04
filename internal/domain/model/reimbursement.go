package model

import "time"

// Reimbursement is one reimbursement request.
type Reimbursement struct {
	ID        string
	RequestNo string
	Applicant string
	TotalYuan Money
	Status    string
	PaidAt    *time.Time
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ReimbursementItem links reimbursement and transaction.
type ReimbursementItem struct {
	ReimbursementID string
	TransactionID   string
	AmountYuan      Money
}
