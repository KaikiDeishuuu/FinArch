package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"finarch/internal/domain/model"
)

// SQLiteTransactionRepository stores transactions in SQLite.
type SQLiteTransactionRepository struct {
	db *sql.DB
}

// NewSQLiteTransactionRepository creates a new transaction repository.
func NewSQLiteTransactionRepository(db *sql.DB) *SQLiteTransactionRepository {
	return &SQLiteTransactionRepository{db: db}
}

// Create inserts one transaction.
func (r *SQLiteTransactionRepository) Create(ctx context.Context, t model.Transaction) error {
	exec := getExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, `
		INSERT INTO transactions (
			id, user_id, occurred_at, direction, source, category, amount_yuan, currency, note,
			project_id, reimbursed, reimbursement_id, created_at, updated_at, uploaded
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		t.ID,
		t.UserID,
		t.OccurredAt.Unix(),
		string(t.Direction),
		string(t.Source),
		t.Category,
		t.AmountYuan.Float64(),
		t.Currency,
		t.Note,
		t.ProjectID,
		boolToInt(t.Reimbursed),
		t.ReimbursementID,
		t.CreatedAt.Unix(),
		t.UpdatedAt.Unix(),
		boolToInt(t.Uploaded),
	)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}
	return nil
}

// ListByUser returns all transactions for the given user ordered by occurred_at desc.
func (r *SQLiteTransactionRepository) ListByUser(ctx context.Context, userID string) ([]model.Transaction, error) {
	exec := getExecutor(ctx, r.db)
	rows, err := exec.QueryContext(ctx, `
		SELECT id, user_id, occurred_at, direction, source, category, amount_yuan, currency, note,
		       project_id, reimbursed, reimbursement_id, created_at, updated_at, uploaded
		FROM transactions
		WHERE user_id = ?
		ORDER BY occurred_at DESC, rowid DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list transactions by user: %w", err)
	}
	defer rows.Close()

	result := make([]model.Transaction, 0)
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}
	return result, nil
}

// GetByIDs loads transactions by IDs.
func (r *SQLiteTransactionRepository) GetByIDs(ctx context.Context, ids []string) ([]model.Transaction, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	exec := getExecutor(ctx, r.db)
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	q := `
		SELECT id, user_id, occurred_at, direction, source, category, amount_yuan, currency, note,
		       project_id, reimbursed, reimbursement_id, created_at, updated_at, uploaded
		FROM transactions
		WHERE id IN (` + placeholders + `)
	`
	rows, err := exec.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query transaction by ids: %w", err)
	}
	defer rows.Close()

	result := make([]model.Transaction, 0, len(ids))
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}
	return result, nil
}

// ListUnreimbursedPersonalExpenses lists unreimbursed personal expenses for a user.
func (r *SQLiteTransactionRepository) ListUnreimbursedPersonalExpenses(ctx context.Context, userID string, projectID *string, maxN int) ([]model.Transaction, error) {
	exec := getExecutor(ctx, r.db)
	args := []any{string(model.SourcePersonal), string(model.DirectionExpense), userID}
	q := `
		SELECT id, user_id, occurred_at, direction, source, category, amount_yuan, currency, note,
		       project_id, reimbursed, reimbursement_id, created_at, updated_at, uploaded
		FROM transactions
		WHERE source = ? AND direction = ? AND reimbursed = 0 AND uploaded = 1 AND user_id = ?
	`
	if projectID != nil {
		q += " AND project_id = ?"
		args = append(args, *projectID)
	}
	q += " ORDER BY amount_yuan DESC, occurred_at ASC"
	if maxN > 0 {
		q += " LIMIT ?"
		args = append(args, maxN)
	}

	rows, err := exec.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query unreimbursed personal expenses: %w", err)
	}
	defer rows.Close()

	result := make([]model.Transaction, 0)
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unreimbursed personal expenses: %w", err)
	}
	return result, nil
}

// MarkReimbursed marks transactions as reimbursed and binds reimbursement ID.
func (r *SQLiteTransactionRepository) MarkReimbursed(ctx context.Context, transactionIDs []string, reimbursementID string) error {
	if len(transactionIDs) == 0 {
		return nil
	}
	exec := getExecutor(ctx, r.db)
	placeholders := strings.TrimRight(strings.Repeat("?,", len(transactionIDs)), ",")
	args := make([]any, 0, len(transactionIDs)+1)
	args = append(args, reimbursementID)
	for _, id := range transactionIDs {
		args = append(args, id)
	}
	q := `
		UPDATE transactions
		SET reimbursed = 1, reimbursement_id = ?, updated_at = strftime('%s','now')
		WHERE reimbursed = 0 AND id IN (` + placeholders + `)
	`
	res, err := exec.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("mark reimbursed: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if int(affected) != len(transactionIDs) {
		return fmt.Errorf("expected %d transactions marked reimbursed, got %d", len(transactionIDs), affected)
	}
	return nil
}

// ToggleReimbursed flips the reimbursed flag for a transaction owned by userID and returns the new state.
func (r *SQLiteTransactionRepository) ToggleReimbursed(ctx context.Context, id string, userID string) (bool, error) {
	exec := getExecutor(ctx, r.db)
	// Read current state, verifying ownership
	var cur int
	var ownerID string
	err := exec.QueryRowContext(ctx,
		`SELECT reimbursed, user_id FROM transactions WHERE id = ?`, id,
	).Scan(&cur, &ownerID)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("transaction %s not found", id)
	}
	if err != nil {
		return false, fmt.Errorf("read reimbursed: %w", err)
	}
	if ownerID != userID {
		return false, fmt.Errorf("permission denied: transaction %s does not belong to you", id)
	}
	newVal := 1 - cur // flip 0↔1
	_, err = exec.ExecContext(ctx,
		`UPDATE transactions SET reimbursed = ?, reimbursement_id = CASE WHEN ? = 0 THEN NULL ELSE reimbursement_id END, updated_at = strftime('%s','now') WHERE id = ?`,
		newVal, newVal, id,
	)
	if err != nil {
		return false, fmt.Errorf("toggle reimbursed: %w", err)
	}
	return newVal == 1, nil
}

// ToggleUploaded flips the uploaded flag for a transaction owned by userID and returns the new state.
func (r *SQLiteTransactionRepository) ToggleUploaded(ctx context.Context, id string, userID string) (bool, error) {
	exec := getExecutor(ctx, r.db)
	var cur int
	var ownerID string
	err := exec.QueryRowContext(ctx,
		`SELECT uploaded, user_id FROM transactions WHERE id = ?`, id,
	).Scan(&cur, &ownerID)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("transaction %s not found", id)
	}
	if err != nil {
		return false, fmt.Errorf("read uploaded: %w", err)
	}
	if ownerID != userID {
		return false, fmt.Errorf("permission denied: transaction %s does not belong to you", id)
	}
	newVal := 1 - cur
	_, err = exec.ExecContext(ctx,
		`UPDATE transactions SET uploaded = ?, updated_at = strftime('%s','now') WHERE id = ?`,
		newVal, id,
	)
	if err != nil {
		return false, fmt.Errorf("toggle uploaded: %w", err)
	}
	return newVal == 1, nil
}

// SumPoolBalance returns company balance and personal outstanding in yuan for a user.
func (r *SQLiteTransactionRepository) SumPoolBalance(ctx context.Context, userID string) (model.Money, model.Money, error) {
	exec := getExecutor(ctx, r.db)

	var companyBalance float64
	if err := exec.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE
			WHEN source = 'company' AND direction = 'income' THEN amount_yuan
			WHEN source = 'company' AND direction = 'expense' THEN -amount_yuan
			ELSE 0 END), 0)
		FROM transactions
		WHERE user_id = ?
	`, userID).Scan(&companyBalance); err != nil {
		return 0, 0, fmt.Errorf("sum company balance: %w", err)
	}

	var personalOutstanding float64
	if err := exec.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE
			WHEN source = 'personal' AND direction = 'expense' AND reimbursed = 0 THEN amount_yuan
			ELSE 0 END), 0)
		FROM transactions
		WHERE user_id = ?
	`, userID).Scan(&personalOutstanding); err != nil {
		return 0, 0, fmt.Errorf("sum personal outstanding: %w", err)
	}

	return model.Money(companyBalance), model.Money(personalOutstanding), nil
}

func scanTransaction(scanner interface {
	Scan(dest ...any) error
}) (model.Transaction, error) {
	var t model.Transaction
	var occurredAt int64
	var direction string
	var source string
	var amount float64
	var projectID sql.NullString
	var reimbursed int
	var reimbursementID sql.NullString
	var createdAt int64
	var updatedAt int64
	var uploaded int

	err := scanner.Scan(
		&t.ID,
		&t.UserID,
		&occurredAt,
		&direction,
		&source,
		&t.Category,
		&amount,
		&t.Currency,
		&t.Note,
		&projectID,
		&reimbursed,
		&reimbursementID,
		&createdAt,
		&updatedAt,
		&uploaded,
	)
	if err != nil {
		return model.Transaction{}, fmt.Errorf("scan transaction: %w", err)
	}

	t.OccurredAt = time.Unix(occurredAt, 0)
	t.Direction = model.Direction(direction)
	t.Source = model.Source(source)
	t.AmountYuan = model.Money(amount)
	t.Reimbursed = reimbursed == 1
	t.Uploaded = uploaded == 1
	t.CreatedAt = time.Unix(createdAt, 0)
	t.UpdatedAt = time.Unix(updatedAt, 0)
	if projectID.Valid {
		v := projectID.String
		t.ProjectID = &v
	}
	if reimbursementID.Valid {
		v := reimbursementID.String
		t.ReimbursementID = &v
	}
	return t, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
