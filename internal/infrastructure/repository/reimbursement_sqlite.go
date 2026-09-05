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
	totalCents := reimbursement.TotalCents
	if totalCents <= 0 {
		var err error
		totalCents, err = reimbursement.TotalYuan.Cents()
		if err != nil || totalCents <= 0 {
			return fmt.Errorf("invalid reimbursement total")
		}
	}
	totalYuan := model.Money(float64(totalCents) / 100.0)
	var paidAt any
	if reimbursement.PaidAt != nil {
		paidAt = reimbursement.PaidAt.Unix()
	}
	_, err := exec.ExecContext(ctx, `
		INSERT INTO reimbursements (id, request_no, applicant, total_yuan, total_cents, status, paid_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, reimbursement.ID, reimbursement.RequestNo, reimbursement.Applicant, totalYuan.Float64(), totalCents, reimbursement.Status, paidAt, reimbursement.CreatedAt.Unix(), reimbursement.UpdatedAt.Unix())
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
		amountCents := item.AmountCents
		if amountCents <= 0 {
			var err error
			amountCents, err = item.AmountYuan.Cents()
			if err != nil || amountCents <= 0 {
				return fmt.Errorf("invalid reimbursement item amount")
			}
		}
		amountYuan := model.Money(float64(amountCents) / 100.0)
		_, err := exec.ExecContext(ctx, `
			INSERT INTO reimbursement_items (reimbursement_id, transaction_id, amount_yuan, amount_cents)
			VALUES (?, ?, ?, ?)
		`, item.ReimbursementID, item.TransactionID, amountYuan.Float64(), amountCents)
		if err != nil {
			return fmt.Errorf("insert reimbursement item: %w", err)
		}
	}
	return nil
}
