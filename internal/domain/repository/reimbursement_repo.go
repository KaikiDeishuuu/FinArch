package repository

import (
	"context"

	"finarch/internal/domain/model"
)

// ReimbursementRepository defines reimbursement persistence behavior.
type ReimbursementRepository interface {
	// Create inserts one reimbursement.
	Create(ctx context.Context, reimbursement model.Reimbursement) error
	// AddItems inserts reimbursement items.
	AddItems(ctx context.Context, items []model.ReimbursementItem) error
}
