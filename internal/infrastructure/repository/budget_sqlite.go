package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"finarch/internal/domain/model"
)

// SQLiteBudgetRepository stores monthly budgets in SQLite.
type SQLiteBudgetRepository struct {
	db *sql.DB
}

// NewSQLiteBudgetRepository creates a new budget repository.
func NewSQLiteBudgetRepository(db *sql.DB) *SQLiteBudgetRepository {
	return &SQLiteBudgetRepository{db: db}
}

const budgetSelectCols = `
	id, user_id, mode, period_month, category,
	amount_cents, currency, base_currency, base_amount_cents,
	is_active, created_at, updated_at`

// Create inserts one active budget.
func (r *SQLiteBudgetRepository) Create(ctx context.Context, b model.Budget) error {
	now := time.Now().UTC().Format(time.RFC3339)
	createdAt := b.CreatedAt.UTC().Format(time.RFC3339)
	if b.CreatedAt.IsZero() {
		createdAt = now
	}
	updatedAt := b.UpdatedAt.UTC().Format(time.RFC3339)
	if b.UpdatedAt.IsZero() {
		updatedAt = now
	}
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO budgets (
			id, user_id, mode, period_month, category,
			amount_cents, currency, base_currency, base_amount_cents,
			is_active, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.UserID, string(b.Mode), b.PeriodMonth, b.Category,
		b.AmountCents, b.Currency, b.BaseCurrency, b.BaseAmountCents,
		boolToInt(b.IsActive), createdAt, updatedAt,
	)
	if err != nil {
		return fmt.Errorf("create budget: %w", err)
	}
	return nil
}

// GetByID loads one active budget by id and owner.
func (r *SQLiteBudgetRepository) GetByID(ctx context.Context, id, userID string) (model.Budget, error) {
	row := getExecutor(ctx, r.db).QueryRowContext(ctx,
		`SELECT`+budgetSelectCols+` FROM budgets WHERE id = ? AND user_id = ? AND is_active = 1`, id, userID)
	return scanBudget(row)
}

// ListByUserMonth returns active budgets for one user/mode/month.
func (r *SQLiteBudgetRepository) ListByUserMonth(ctx context.Context, userID string, mode model.Mode, periodMonth string) ([]model.Budget, error) {
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx,
		`SELECT`+budgetSelectCols+` FROM budgets
		 WHERE user_id = ? AND mode = ? AND period_month = ? AND is_active = 1
		 ORDER BY CASE WHEN category = '' THEN 0 ELSE 1 END, category`,
		userID, string(mode), periodMonth)
	if err != nil {
		return nil, fmt.Errorf("list budgets: %w", err)
	}
	defer rows.Close()

	budgets := []model.Budget{}
	for rows.Next() {
		b, err := scanBudget(rows)
		if err != nil {
			return nil, err
		}
		budgets = append(budgets, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate budgets: %w", err)
	}
	return budgets, nil
}

// Update saves mutable fields on an active budget.
func (r *SQLiteBudgetRepository) Update(ctx context.Context, b model.Budget) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		UPDATE budgets
		SET period_month = ?, category = ?, amount_cents = ?, currency = ?,
		    base_currency = ?, base_amount_cents = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND is_active = 1`,
		b.PeriodMonth, b.Category, b.AmountCents, b.Currency,
		b.BaseCurrency, b.BaseAmountCents, time.Now().UTC().Format(time.RFC3339),
		b.ID, b.UserID,
	)
	if err != nil {
		return fmt.Errorf("update budget: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("budget not found")
	}
	return nil
}

// Delete deactivates a budget.
func (r *SQLiteBudgetRepository) Delete(ctx context.Context, id, userID string) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE budgets SET is_active = 0, updated_at = ? WHERE id = ? AND user_id = ? AND is_active = 1`,
		time.Now().UTC().Format(time.RFC3339), id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete budget: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("budget not found")
	}
	return nil
}

// GetMonthlyExpenseActuals sums expense transactions for one user/mode/month.
func (r *SQLiteBudgetRepository) GetMonthlyExpenseActuals(ctx context.Context, userID string, mode model.Mode, periodMonth string) (model.BudgetActuals, error) {
	start, err := time.Parse("2006-01", periodMonth)
	if err != nil {
		return model.BudgetActuals{}, fmt.Errorf("invalid budget month: %w", err)
	}
	from := start.Format("2006-01-02")
	to := start.AddDate(0, 1, 0).Format("2006-01-02")
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx, `
		SELECT COALESCE(category, ''), COALESCE(SUM(base_amount_cents), 0)
		FROM transactions
		WHERE user_id = ?
		  AND mode = ?
		  AND type = 'expense'
		  AND txn_date >= ?
		  AND txn_date < ?
		GROUP BY COALESCE(category, '')`, userID, string(mode), from, to)
	if err != nil {
		return model.BudgetActuals{}, fmt.Errorf("budget actuals: %w", err)
	}
	defer rows.Close()

	actuals := model.BudgetActuals{ByCategory: map[string]int64{}}
	for rows.Next() {
		var category string
		var cents int64
		if err := rows.Scan(&category, &cents); err != nil {
			return model.BudgetActuals{}, fmt.Errorf("scan budget actuals: %w", err)
		}
		actuals.ByCategory[category] = cents
		actuals.TotalCents += cents
	}
	if err := rows.Err(); err != nil {
		return model.BudgetActuals{}, fmt.Errorf("iterate budget actuals: %w", err)
	}
	return actuals, nil
}

type budgetScanner interface {
	Scan(dest ...any) error
}

func scanBudget(s budgetScanner) (model.Budget, error) {
	var b model.Budget
	var mode string
	var isActive int
	var createdAt, updatedAt string
	if err := s.Scan(
		&b.ID, &b.UserID, &mode, &b.PeriodMonth, &b.Category,
		&b.AmountCents, &b.Currency, &b.BaseCurrency, &b.BaseAmountCents,
		&isActive, &createdAt, &updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.Budget{}, fmt.Errorf("budget not found")
		}
		return model.Budget{}, fmt.Errorf("scan budget: %w", err)
	}
	b.Mode = model.Mode(mode)
	b.IsActive = isActive == 1
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		b.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		b.UpdatedAt = t
	}
	return b, nil
}
