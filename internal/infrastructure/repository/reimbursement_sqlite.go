package repository

import (
	"context"
	"database/sql"
	"fmt"

	"finarch/internal/domain/model"
)

// SQLiteReimbursementRepository stores reimbursements in SQLite.
type SQLiteReimbursementRepository struct {
	db *sql.DB
}

// NewSQLiteReimbursementRepository creates a new reimbursement repository.
func NewSQLiteReimbursementRepository(db *sql.DB) *SQLiteReimbursementRepository {
	return &SQLiteReimbursementRepository{db: db}
}

// Create inserts one reimbursement.
func (r *SQLiteReimbursementRepository) Create(ctx context.Context, reimbursement model.Reimbursement) error {
	exec := getExecutor(ctx, r.db)
	var paidAt any
	if reimbursement.PaidAt != nil {
		paidAt = reimbursement.PaidAt.Unix()
	}
	_, err := exec.ExecContext(ctx, `
		INSERT INTO reimbursements (id, request_no, applicant, total_yuan, status, paid_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, reimbursement.ID, reimbursement.RequestNo, reimbursement.Applicant, reimbursement.TotalYuan.Float64(), reimbursement.Status, paidAt, reimbursement.CreatedAt.Unix(), reimbursement.UpdatedAt.Unix())
	if err != nil {
		return fmt.Errorf("insert reimbursement: %w", err)
	}
	return nil
}

// AddItems inserts reimbursement items.
func (r *SQLiteReimbursementRepository) AddItems(ctx context.Context, items []model.ReimbursementItem) error {
	if len(items) == 0 {
		return nil
	}
	exec := getExecutor(ctx, r.db)
	for _, item := range items {
		_, err := exec.ExecContext(ctx, `
			INSERT INTO reimbursement_items (reimbursement_id, transaction_id, amount_yuan)
			VALUES (?, ?, ?)
		`, item.ReimbursementID, item.TransactionID, item.AmountYuan.Float64())
		if err != nil {
			return fmt.Errorf("insert reimbursement item: %w", err)
		}
	}
	return nil
}
