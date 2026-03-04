package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"finarch/internal/domain/model"
)

// SQLiteAccountRepository stores accounts in SQLite.
type SQLiteAccountRepository struct {
	db *sql.DB
}

// NewSQLiteAccountRepository creates a new account repository.
func NewSQLiteAccountRepository(db *sql.DB) *SQLiteAccountRepository {
	return &SQLiteAccountRepository{db: db}
}

const accountSelectCols = `
  id, user_id, name, type, currency,
  balance_cents, version, is_active,
  created_at, updated_at`

// Create inserts one account.
func (r *SQLiteAccountRepository) Create(ctx context.Context, a model.Account) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO accounts (id, user_id, name, type, currency, balance_cents, version, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.UserID, a.Name, string(a.Type), a.Currency,
		a.BalanceCents, a.Version, boolToInt(a.IsActive),
		a.CreatedAt.UTC().Format(time.RFC3339),
		a.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}
	return nil
}

// GetByID loads an account by primary key.
func (r *SQLiteAccountRepository) GetByID(ctx context.Context, id string) (model.Account, error) {
	row := getExecutor(ctx, r.db).QueryRowContext(ctx,
		`SELECT`+accountSelectCols+` FROM accounts WHERE id = ?`, id)
	return scanAccount(row)
}

// ListByUser returns all active accounts for a user.
func (r *SQLiteAccountRepository) ListByUser(ctx context.Context, userID string) ([]model.Account, error) {
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx,
		`SELECT`+accountSelectCols+` FROM accounts WHERE user_id = ? AND is_active = 1 ORDER BY type, name`, userID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()
	var out []model.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetByUserAndType returns the first active account of the given type for a user.
func (r *SQLiteAccountRepository) GetByUserAndType(ctx context.Context, userID string, t model.AccountType) (model.Account, error) {
	row := getExecutor(ctx, r.db).QueryRowContext(ctx,
		`SELECT`+accountSelectCols+` FROM accounts WHERE user_id = ? AND type = ? AND is_active = 1 LIMIT 1`,
		userID, string(t))
	return scanAccount(row)
}

// Update persists name and is_active changes with optimistic locking.
func (r *SQLiteAccountRepository) Update(ctx context.Context, a model.Account) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE accounts SET name = ?, is_active = ?, updated_at = ?, version = version + 1
		 WHERE id = ? AND user_id = ? AND version = ?`,
		a.Name, boolToInt(a.IsActive),
		time.Now().UTC().Format(time.RFC3339),
		a.ID, a.UserID, a.Version,
	)
	if err != nil {
		return fmt.Errorf("update account: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("concurrent_modification")
	}
	return nil
}

// UpdateName saves only the name of an account atomically.
func (r *SQLiteAccountRepository) UpdateName(ctx context.Context, id, userID, newName string) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE accounts SET name = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		newName,
		time.Now().UTC().Format(time.RFC3339),
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("update account name: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("account not found or not owned by user")
	}
	return nil
}

// Delete soft-deletes an account by setting is_active = 0.
func (r *SQLiteAccountRepository) Delete(ctx context.Context, id, userID string) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE accounts SET is_active = 0, updated_at = ? WHERE id = ? AND user_id = ? AND is_active = 1`,
		time.Now().UTC().Format(time.RFC3339), id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("account not found")
	}
	return nil
}

// CountByUserAndType returns how many active accounts of the given type a user has.
func (r *SQLiteAccountRepository) CountByUserAndType(ctx context.Context, userID string, t model.AccountType) (int, error) {
	var count int
	err := getExecutor(ctx, r.db).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE user_id = ? AND type = ? AND is_active = 1`,
		userID, string(t),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count accounts: %w", err)
	}
	return count, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

type accountScanner interface {
	Scan(dest ...any) error
}

func scanAccount(s accountScanner) (model.Account, error) {
	var a model.Account
	var typ string
	var isActive int
	var createdAt, updatedAt string
	if err := s.Scan(
		&a.ID, &a.UserID, &a.Name, &typ, &a.Currency,
		&a.BalanceCents, &a.Version, &isActive,
		&createdAt, &updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.Account{}, fmt.Errorf("account not found")
		}
		return model.Account{}, fmt.Errorf("scan account: %w", err)
	}
	a.Type = model.AccountType(typ)
	a.IsActive = isActive == 1
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		a.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		a.UpdatedAt = t
	}
	return a, nil
}
