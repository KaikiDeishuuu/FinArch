package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"finarch/internal/domain/model"
)

// SQLiteTransactionRepository stores transactions in SQLite (V9 schema).
type SQLiteTransactionRepository struct {
	db *sql.DB
}

// NewSQLiteTransactionRepository creates a new transaction repository.
func NewSQLiteTransactionRepository(db *sql.DB) *SQLiteTransactionRepository {
	return &SQLiteTransactionRepository{db: db}
}

// txnSelectSQL selects all columns needed to populate model.Transaction,
// including a JOIN with accounts for the backward-compat Source field.
const txnSelectSQL = `
  SELECT t.id, t.user_id, t.group_id,
         t.direction, t.account_id,
         t.amount_cents, t.currency, t.exchange_rate, t.base_amount_cents,
         t.type, t.category_id, t.category,
         t.reimb_status, t.reimb_to_account, t.reimbursement_id,
         t.project_id, t.project,
         t.mode, t.note, t.uploaded, t.txn_date,
         t.created_at, t.updated_at,
         COALESCE(a.type, 'personal') AS account_type
  FROM transactions t
  LEFT JOIN accounts a ON a.id = t.account_id`

// Create inserts one transaction (the balance trigger fires automatically).
func (r *SQLiteTransactionRepository) Create(ctx context.Context, t model.Transaction) error {
	exec := getExecutor(ctx, r.db)
	if t.TxnDate == "" {
		t.TxnDate = t.OccurredAt.Format("2006-01-02")
	}
	if t.GroupID == "" {
		t.GroupID = t.ID
	}
	ledgerDir := t.LedgerDir
	if ledgerDir == "" {
		if t.TxType == model.TxTypeIncome {
			ledgerDir = model.LedgerCredit
		} else {
			ledgerDir = model.LedgerDebit
		}
	}
	txType := t.TxType
	if txType == "" {
		txType = model.TxType(t.Direction) // backward compat: income/expense
	}
	exchangeRate := t.ExchangeRate
	if exchangeRate == 0 {
		exchangeRate = 1.0
	}
	amountCents := t.AmountCents
	if amountCents == 0 && t.AmountYuan != 0 {
		amountCents = int64(t.AmountYuan * 100)
	}
	baseAmountCents := t.BaseAmountCents
	if baseAmountCents == 0 {
		baseAmountCents = int64(float64(amountCents) * exchangeRate)
	}
	reimb := t.ReimbStatus
	if t.Mode == "" {
		t.Mode = model.ModeWork
	}
	if reimb == "" {
		if t.Reimbursed {
			reimb = model.ReimbStatusReimbursed
		} else {
			reimb = model.ReimbStatusNone
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := exec.ExecContext(ctx, `
		INSERT INTO transactions (
			id, user_id, group_id, direction, account_id,
			amount_cents, currency, exchange_rate, base_amount_cents,
			type, category_id, category,
			reimb_status, reimb_to_account, reimbursement_id,
			project_id, project,
			mode, note, uploaded, idempotency_key, txn_date, created_at, updated_at
		) VALUES (
			?,?,?,?,?,
			?,?,?,?,
			?,?,?,
			?,?,?,
			?,?,
			?,?,?,?,?,?,?
		)`,
		t.ID, t.UserID, t.GroupID, string(ledgerDir), t.AccountID,
		amountCents, t.Currency, exchangeRate, baseAmountCents,
		string(txType), t.CategoryID, t.Category,
		string(reimb), t.ReimbToAccount, t.ReimbursementID,
		t.ProjectID, t.Project,
		string(t.Mode), t.Note, boolToInt(t.Uploaded), t.IdempotencyKey, t.TxnDate, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}
	return nil
}

// ListByUser returns all transactions for the given user ordered by txn_date desc.
func (r *SQLiteTransactionRepository) ListByUser(ctx context.Context, userID string, mode model.Mode) ([]model.Transaction, error) {
	exec := getExecutor(ctx, r.db)
	rows, err := exec.QueryContext(ctx,
		txnSelectSQL+` WHERE t.user_id = ? AND t.mode = ? ORDER BY t.txn_date DESC, t.rowid DESC`, userID, string(mode))
	if err != nil {
		return nil, fmt.Errorf("list transactions by user: %w", err)
	}
	defer rows.Close()
	return collectTransactions(rows)
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
	rows, err := exec.QueryContext(ctx,
		txnSelectSQL+` WHERE t.id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("query transactions by ids: %w", err)
	}
	defer rows.Close()
	return collectTransactions(rows)
}

// ListUnreimbursedPersonalExpenses lists personal expenses with reimb_status='pending'.
// Hits the partial index idx_txn_pending_reimb.
func (r *SQLiteTransactionRepository) ListUnreimbursedPersonalExpenses(ctx context.Context, userID string, projectID *string, maxN int, mode model.Mode) ([]model.Transaction, error) {
	exec := getExecutor(ctx, r.db)
	args := []any{userID, string(mode)}
	q := txnSelectSQL + `
		WHERE t.user_id = ?
		  AND t.mode = ?
		  AND t.reimb_status = 'pending'
		  AND t.uploaded = 1
		  AND t.type = 'expense'`
	if projectID != nil {
		q += " AND t.project_id = ?"
		args = append(args, *projectID)
	}
	q += " ORDER BY t.base_amount_cents DESC, t.txn_date ASC"
	if maxN > 0 {
		q += " LIMIT ?"
		args = append(args, maxN)
	}
	rows, err := exec.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query unreimbursed personal expenses: %w", err)
	}
	defer rows.Close()
	return collectTransactions(rows)
}

// MarkReimbursed sets reimb_status='reimbursed' for the given transaction IDs.
func (r *SQLiteTransactionRepository) MarkReimbursed(ctx context.Context, transactionIDs []string, reimbursementID string) error {
	if len(transactionIDs) == 0 {
		return nil
	}
	exec := getExecutor(ctx, r.db)
	placeholders := strings.TrimRight(strings.Repeat("?,", len(transactionIDs)), ",")
	args := []any{reimbursementID}
	for _, id := range transactionIDs {
		args = append(args, id)
	}
	q := `UPDATE transactions
		SET reimb_status = 'reimbursed', reimbursement_id = ?,
		    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE reimb_status = 'pending' AND id IN (` + placeholders + `)`
	res, err := exec.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("mark reimbursed: %w", err)
	}
	if n, _ := res.RowsAffected(); int(n) != len(transactionIDs) {
		return fmt.Errorf("expected %d rows marked reimbursed, got %d", len(transactionIDs), n)
	}
	return nil
}

// ToggleReimbursed flips reimb_status between 'pending' and 'reimbursed'.
func (r *SQLiteTransactionRepository) ToggleReimbursed(ctx context.Context, id string, userID string) (bool, error) {
	exec := getExecutor(ctx, r.db)
	var cur string
	var ownerID string
	if err := exec.QueryRowContext(ctx,
		`SELECT reimb_status, user_id FROM transactions WHERE id = ?`, id,
	).Scan(&cur, &ownerID); err != nil {
		if err == sql.ErrNoRows {
			return false, fmt.Errorf("交易记录不存在")
		}
		return false, fmt.Errorf("read reimb_status: %w", err)
	}
	if ownerID != userID {
		return false, fmt.Errorf("无权操作该交易")
	}
	var newStatus string
	if cur == string(model.ReimbStatusReimbursed) {
		newStatus = string(model.ReimbStatusPending)
	} else {
		newStatus = string(model.ReimbStatusReimbursed)
	}
	nowStr := time.Now().UTC().Format(time.RFC3339)
	if _, err := exec.ExecContext(ctx,
		`UPDATE transactions SET reimb_status = ?,
		  reimbursement_id = CASE WHEN ? = 'pending' THEN NULL ELSE reimbursement_id END,
		  updated_at = ?
		 WHERE id = ?`,
		newStatus, newStatus, nowStr, id,
	); err != nil {
		return false, fmt.Errorf("toggle reimbursed: %w", err)
	}
	return newStatus == string(model.ReimbStatusReimbursed), nil
}

// ToggleUploaded flips the uploaded flag.
func (r *SQLiteTransactionRepository) ToggleUploaded(ctx context.Context, id string, userID string) (bool, error) {
	exec := getExecutor(ctx, r.db)
	var cur int
	var ownerID string
	if err := exec.QueryRowContext(ctx,
		`SELECT uploaded, user_id FROM transactions WHERE id = ?`, id,
	).Scan(&cur, &ownerID); err != nil {
		if err == sql.ErrNoRows {
			return false, fmt.Errorf("交易记录不存在")
		}
		return false, fmt.Errorf("read uploaded: %w", err)
	}
	if ownerID != userID {
		return false, fmt.Errorf("无权操作该交易")
	}
	newVal := 1 - cur
	nowStr := time.Now().UTC().Format(time.RFC3339)
	if _, err := exec.ExecContext(ctx,
		`UPDATE transactions SET uploaded = ?, updated_at = ? WHERE id = ?`,
		newVal, nowStr, id,
	); err != nil {
		return false, fmt.Errorf("toggle uploaded: %w", err)
	}
	return newVal == 1, nil
}

// SumPoolBalance returns public-account net balance and pending personal reimbursements.
// Uses cached balance in accounts table (O(1) reads).
func (r *SQLiteTransactionRepository) SumPoolBalance(ctx context.Context, userID string, mode model.Mode) (model.Money, model.Money, error) {
	exec := getExecutor(ctx, r.db)

	var publicCents int64
	if err := exec.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(balance_cents),0) FROM accounts WHERE user_id=? AND type='public' AND is_active=1`, userID,
	).Scan(&publicCents); err != nil {
		return 0, 0, fmt.Errorf("sum public balance: %w", err)
	}

	var pendingCents int64
	if err := exec.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(base_amount_cents),0) FROM transactions WHERE user_id=? AND mode=? AND reimb_status='pending'`, userID, string(mode),
	).Scan(&pendingCents); err != nil {
		return 0, 0, fmt.Errorf("sum pending reimb: %w", err)
	}

	return model.Money(float64(publicCents) / 100.0),
		model.Money(float64(pendingCents) / 100.0),
		nil
}

// HasUnreimbursedByAccount returns true when the given account has at least one
// expense transaction that has not yet been reimbursed (reimb_status='pending').
// This is used to guard sub-account deletion so no pending expense is orphaned.
func (r *SQLiteTransactionRepository) HasUnreimbursedByAccount(ctx context.Context, accountID, userID string) (bool, error) {
	exec := getExecutor(ctx, r.db)
	var count int
	err := exec.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transactions
		 WHERE account_id = ? AND user_id = ?
		   AND type = 'expense' AND reimb_status = 'pending'`,
		accountID, userID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check unreimbursed by account: %w", err)
	}
	return count > 0, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func collectTransactions(rows *sql.Rows) ([]model.Transaction, error) {
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

func scanTransaction(scanner interface {
	Scan(dest ...any) error
}) (model.Transaction, error) {
	var t model.Transaction
	var ledgerDir, txType, reimbStatus, accountType, mode string
	var categoryID, reimbToAccount, reimbursementID sql.NullString
	var projectID sql.NullString
	var project sql.NullString
	var createdAt, updatedAt string
	var uploaded int

	if err := scanner.Scan(
		&t.ID, &t.UserID, &t.GroupID,
		&ledgerDir, &t.AccountID,
		&t.AmountCents, &t.Currency, &t.ExchangeRate, &t.BaseAmountCents,
		&txType, &categoryID, &t.Category,
		&reimbStatus, &reimbToAccount, &reimbursementID,
		&projectID, &project,
		&mode, &t.Note, &uploaded, &t.TxnDate,
		&createdAt, &updatedAt,
		&accountType,
	); err != nil {
		return model.Transaction{}, fmt.Errorf("scan transaction: %w", err)
	}

	t.LedgerDir = model.LedgerDir(ledgerDir)
	t.TxType = model.TxType(txType)
	t.ReimbStatus = model.ReimbStatus(reimbStatus)
	t.AccountType = model.AccountType(accountType)
	t.Mode = model.Mode(mode)
	t.Uploaded = uploaded == 1
	t.AmountYuan = model.Money(float64(t.AmountCents) / 100.0)
	t.Reimbursed = t.ReimbStatus == model.ReimbStatusReimbursed

	// Backward-compat derived fields
	if t.TxType == model.TxTypeExpense {
		t.Direction = model.DirectionExpense
	} else {
		t.Direction = model.DirectionIncome
	}
	if t.AccountType == model.AccountTypePersonal {
		t.Source = model.SourcePersonal
	} else {
		t.Source = model.SourceCompany
	}

	// Parse txn_date → OccurredAt (midnight UTC)
	if d, err := time.ParseInLocation("2006-01-02", t.TxnDate, time.UTC); err == nil {
		t.OccurredAt = d
	}

	if categoryID.Valid {
		v := categoryID.String
		t.CategoryID = &v
	}
	if reimbToAccount.Valid {
		v := reimbToAccount.String
		t.ReimbToAccount = &v
	}
	if reimbursementID.Valid {
		v := reimbursementID.String
		t.ReimbursementID = &v
	}
	if projectID.Valid {
		v := projectID.String
		t.ProjectID = &v
	}
	if project.Valid {
		v := project.String
		t.Project = &v
	}

	if ts, err := time.Parse(time.RFC3339, createdAt); err == nil {
		t.CreatedAt = ts
	}
	if ts, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		t.UpdatedAt = ts
	}
	return t, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
