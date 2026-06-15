package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"finarch/internal/domain/model"
)

// SQLiteRecurringTransactionRepository stores recurring transaction rules.
type SQLiteRecurringTransactionRepository struct {
	db *sql.DB
}

// NewSQLiteRecurringTransactionRepository creates a recurring repository.
func NewSQLiteRecurringTransactionRepository(db *sql.DB) *SQLiteRecurringTransactionRepository {
	return &SQLiteRecurringTransactionRepository{db: db}
}

const recurringRuleSelectCols = `
	id, user_id, mode, name, status, account_id, type, category,
	amount_cents, currency, exchange_rate, note, project_id,
	frequency, interval, start_date, end_date, time_of_day, timezone,
	day_of_week, day_of_month, month_end_policy, next_run_at,
	last_generated_for, catch_up_enabled, created_at, updated_at`

func (r *SQLiteRecurringTransactionRepository) CreateRule(ctx context.Context, rule model.RecurringTransactionRule) error {
	_, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO recurring_transaction_rules (
			id, user_id, mode, name, status, account_id, type, category,
			amount_cents, currency, exchange_rate, note, project_id,
			frequency, interval, start_date, end_date, time_of_day, timezone,
			day_of_week, day_of_month, month_end_policy, next_run_at,
			last_generated_for, catch_up_enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.UserID, string(rule.Mode), rule.Name, string(rule.Status), rule.AccountID, string(rule.TxType), rule.Category,
		rule.AmountCents, rule.Currency, rule.ExchangeRate, rule.Note, rule.ProjectID,
		string(rule.Frequency), rule.Interval, rule.StartDate, rule.EndDate, rule.TimeOfDay, rule.Timezone,
		rule.DayOfWeek, rule.DayOfMonth, string(rule.MonthEndPolicy), rule.NextRunAt,
		rule.LastGeneratedFor, boolToInt(rule.CatchUpEnabled), formatRepoTime(rule.CreatedAt), formatRepoTime(rule.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("create recurring rule: %w", err)
	}
	return nil
}

func (r *SQLiteRecurringTransactionRepository) GetRuleByID(ctx context.Context, id, userID string) (model.RecurringTransactionRule, error) {
	row := getExecutor(ctx, r.db).QueryRowContext(ctx,
		`SELECT`+recurringRuleSelectCols+` FROM recurring_transaction_rules WHERE id = ? AND user_id = ?`, id, userID)
	return scanRecurringRule(row)
}

func (r *SQLiteRecurringTransactionRepository) ListRulesByUser(ctx context.Context, userID string, mode model.Mode) ([]model.RecurringTransactionRule, error) {
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx,
		`SELECT`+recurringRuleSelectCols+` FROM recurring_transaction_rules
		 WHERE user_id = ? AND mode = ? AND status != 'ended'
		 ORDER BY status, next_run_at ASC, created_at DESC`, userID, string(mode))
	if err != nil {
		return nil, fmt.Errorf("list recurring rules: %w", err)
	}
	defer rows.Close()
	return collectRecurringRules(rows)
}

func (r *SQLiteRecurringTransactionRepository) UpdateRule(ctx context.Context, rule model.RecurringTransactionRule) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		UPDATE recurring_transaction_rules
		SET mode = ?, name = ?, status = ?, account_id = ?, type = ?, category = ?,
		    amount_cents = ?, currency = ?, exchange_rate = ?, note = ?, project_id = ?,
		    frequency = ?, interval = ?, start_date = ?, end_date = ?, time_of_day = ?, timezone = ?,
		    day_of_week = ?, day_of_month = ?, month_end_policy = ?, next_run_at = ?,
		    last_generated_for = ?, catch_up_enabled = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		string(rule.Mode), rule.Name, string(rule.Status), rule.AccountID, string(rule.TxType), rule.Category,
		rule.AmountCents, rule.Currency, rule.ExchangeRate, rule.Note, rule.ProjectID,
		string(rule.Frequency), rule.Interval, rule.StartDate, rule.EndDate, rule.TimeOfDay, rule.Timezone,
		rule.DayOfWeek, rule.DayOfMonth, string(rule.MonthEndPolicy), rule.NextRunAt,
		rule.LastGeneratedFor, boolToInt(rule.CatchUpEnabled), time.Now().UTC().Format(time.RFC3339),
		rule.ID, rule.UserID,
	)
	if err != nil {
		return fmt.Errorf("update recurring rule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("recurring rule not found")
	}
	return nil
}

func (r *SQLiteRecurringTransactionRepository) DeleteRule(ctx context.Context, id, userID string) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE recurring_transaction_rules SET status = 'ended', updated_at = ? WHERE id = ? AND user_id = ?`,
		time.Now().UTC().Format(time.RFC3339), id, userID)
	if err != nil {
		return fmt.Errorf("delete recurring rule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("recurring rule not found")
	}
	return nil
}

func (r *SQLiteRecurringTransactionRepository) ListDueRules(ctx context.Context, nowUnix int64, limit int) ([]model.RecurringTransactionRule, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx,
		`SELECT`+recurringRuleSelectCols+` FROM recurring_transaction_rules
		 WHERE status = 'active' AND next_run_at <= ?
		 ORDER BY next_run_at ASC, id ASC LIMIT ?`, nowUnix, limit)
	if err != nil {
		return nil, fmt.Errorf("list due recurring rules: %w", err)
	}
	defer rows.Close()
	return collectRecurringRules(rows)
}

func (r *SQLiteRecurringTransactionRepository) ClaimInstance(ctx context.Context, inst model.RecurringTransactionInstance) (bool, error) {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx, `
		INSERT OR IGNORE INTO recurring_transaction_instances (
			id, rule_id, user_id, occurrence_date, scheduled_at, transaction_id,
			idempotency_key, status, error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inst.ID, inst.RuleID, inst.UserID, inst.OccurrenceDate, inst.ScheduledAt, inst.TransactionID,
		inst.IdempotencyKey, string(inst.Status), inst.Error, formatRepoTime(inst.CreatedAt), formatRepoTime(inst.UpdatedAt),
	)
	if err != nil {
		return false, fmt.Errorf("claim recurring instance: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *SQLiteRecurringTransactionRepository) MarkInstanceGenerated(ctx context.Context, id, transactionID string) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE recurring_transaction_instances SET status = 'generated', transaction_id = ?, error = NULL, updated_at = ? WHERE id = ?`,
		transactionID, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("mark recurring generated: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("recurring instance not found")
	}
	return nil
}

func (r *SQLiteRecurringTransactionRepository) MarkInstanceFailed(ctx context.Context, id, message string) error {
	res, err := getExecutor(ctx, r.db).ExecContext(ctx,
		`UPDATE recurring_transaction_instances SET status = 'failed', error = ?, updated_at = ? WHERE id = ?`,
		message, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("mark recurring failed: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("recurring instance not found")
	}
	return nil
}

func (r *SQLiteRecurringTransactionRepository) ListInstances(ctx context.Context, ruleID, userID string, limit int) ([]model.RecurringTransactionInstance, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := getExecutor(ctx, r.db).QueryContext(ctx, `
		SELECT id, rule_id, user_id, occurrence_date, scheduled_at, transaction_id,
		       idempotency_key, status, error, created_at, updated_at
		FROM recurring_transaction_instances
		WHERE rule_id = ? AND user_id = ?
		ORDER BY scheduled_at DESC, created_at DESC LIMIT ?`, ruleID, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recurring instances: %w", err)
	}
	defer rows.Close()
	return collectRecurringInstances(rows)
}

func collectRecurringRules(rows *sql.Rows) ([]model.RecurringTransactionRule, error) {
	out := []model.RecurringTransactionRule{}
	for rows.Next() {
		rule, err := scanRecurringRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

type recurringRuleScanner interface{ Scan(dest ...any) error }

func scanRecurringRule(s recurringRuleScanner) (model.RecurringTransactionRule, error) {
	var r model.RecurringTransactionRule
	var mode, status, txType, frequency, monthEndPolicy string
	var projectID, endDate, lastGeneratedFor sql.NullString
	var dayOfWeek, dayOfMonth sql.NullInt64
	var catchUp int
	var createdAt, updatedAt string
	if err := s.Scan(
		&r.ID, &r.UserID, &mode, &r.Name, &status, &r.AccountID, &txType, &r.Category,
		&r.AmountCents, &r.Currency, &r.ExchangeRate, &r.Note, &projectID,
		&frequency, &r.Interval, &r.StartDate, &endDate, &r.TimeOfDay, &r.Timezone,
		&dayOfWeek, &dayOfMonth, &monthEndPolicy, &r.NextRunAt,
		&lastGeneratedFor, &catchUp, &createdAt, &updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.RecurringTransactionRule{}, fmt.Errorf("recurring rule not found")
		}
		return model.RecurringTransactionRule{}, fmt.Errorf("scan recurring rule: %w", err)
	}
	r.Mode = model.Mode(mode)
	r.Status = model.RecurringRuleStatus(status)
	r.TxType = model.TxType(txType)
	r.Frequency = model.RecurringFrequency(frequency)
	r.MonthEndPolicy = model.MonthEndPolicy(monthEndPolicy)
	r.CatchUpEnabled = catchUp == 1
	if projectID.Valid {
		v := projectID.String
		r.ProjectID = &v
	}
	if endDate.Valid {
		v := endDate.String
		r.EndDate = &v
	}
	if dayOfWeek.Valid {
		v := int(dayOfWeek.Int64)
		r.DayOfWeek = &v
	}
	if dayOfMonth.Valid {
		v := int(dayOfMonth.Int64)
		r.DayOfMonth = &v
	}
	if lastGeneratedFor.Valid {
		v := lastGeneratedFor.String
		r.LastGeneratedFor = &v
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		r.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		r.UpdatedAt = t
	}
	return r, nil
}

func collectRecurringInstances(rows *sql.Rows) ([]model.RecurringTransactionInstance, error) {
	out := []model.RecurringTransactionInstance{}
	for rows.Next() {
		inst, err := scanRecurringInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}

type recurringInstanceScanner interface{ Scan(dest ...any) error }

func scanRecurringInstance(s recurringInstanceScanner) (model.RecurringTransactionInstance, error) {
	var inst model.RecurringTransactionInstance
	var status string
	var transactionID, errMsg sql.NullString
	var createdAt, updatedAt string
	if err := s.Scan(&inst.ID, &inst.RuleID, &inst.UserID, &inst.OccurrenceDate, &inst.ScheduledAt, &transactionID,
		&inst.IdempotencyKey, &status, &errMsg, &createdAt, &updatedAt); err != nil {
		return model.RecurringTransactionInstance{}, fmt.Errorf("scan recurring instance: %w", err)
	}
	inst.Status = model.RecurringInstanceStatus(status)
	if transactionID.Valid {
		v := transactionID.String
		inst.TransactionID = &v
	}
	if errMsg.Valid {
		v := errMsg.String
		inst.Error = &v
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		inst.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		inst.UpdatedAt = t
	}
	return inst, nil
}

func formatRepoTime(t time.Time) string {
	if t.IsZero() {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return t.UTC().Format(time.RFC3339)
}
