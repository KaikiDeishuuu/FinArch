package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"finarch/internal/domain/model"
)

// StatsService provides aggregated financial statistics (V9 schema).
type StatsService struct {
	db *sql.DB
}

func NewStatsService(db *sql.DB) *StatsService {
	return &StatsService{db: db}
}

// PoolBalance is a single fund-pool balance summary.
type PoolBalance struct {
	CompanyBalance      float64 `json:"company_balance"`
	PersonalOutstanding float64 `json:"personal_outstanding"`
}

// MonthlyStat is one month's income/expense totals.
type MonthlyStat struct {
	Year       int     `json:"year"`
	Month      int     `json:"month"`
	Income     float64 `json:"income"`
	Expense    float64 `json:"expense"`
	Reimbursed float64 `json:"reimbursed"` // 已报销的个人垫付金额
}

// CategoryStat is per-category expense total.
type CategoryStat struct {
	Category string  `json:"category"`
	Total    float64 `json:"total"`
	Count    int     `json:"count"`
}

// ProjectStat is per-project totals.
type ProjectStat struct {
	ProjectID   string  `json:"project_id"`
	ProjectName string  `json:"project_name"`
	Income      float64 `json:"income"`
	Expense     float64 `json:"expense"`
	Net         float64 `json:"net"`
}

type BalanceHistoryPoint struct {
	Date    string  `json:"date"`
	Balance float64 `json:"balance"`
}

// Summary returns company balance + personal outstanding for a user.
// Reads from accounts.balance_cents (O(1)) and the pending-reimb partial index.
func (s *StatsService) Summary(ctx context.Context, userID string) (PoolBalance, error) {
	var b PoolBalance

	// Public accounts: read cached balance (trigger-maintained)
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(balance_cents), 0)
		FROM accounts
		WHERE user_id = ? AND type = 'public' AND is_active = 1
	`, userID).Scan(&b.CompanyBalance); err != nil {
		return PoolBalance{}, fmt.Errorf("company balance: %w", err)
	}
	b.CompanyBalance /= 100.0

	// Personal outstanding: hits partial index idx_txn_pending_reimb
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(base_amount_cents), 0)
		FROM transactions
		WHERE user_id = ? AND reimb_status = 'pending'
	`, userID).Scan(&b.PersonalOutstanding); err != nil {
		return PoolBalance{}, fmt.Errorf("personal outstanding: %w", err)
	}
	b.PersonalOutstanding /= 100.0

	return b, nil
}

// Monthly returns income/expense grouped by month for a given year for a user.
func (s *StatsService) Monthly(ctx context.Context, userID string, year int) ([]MonthlyStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
		  CAST(substr(txn_date, 1, 4) AS INTEGER)  AS yr,
		  CAST(substr(txn_date, 6, 2) AS INTEGER)  AS mo,
		  COALESCE(SUM(CASE WHEN type = 'income'  THEN CAST(base_amount_cents AS REAL) / 100 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN type = 'expense' THEN CAST(base_amount_cents AS REAL) / 100 ELSE 0 END), 0),
		  COALESCE(SUM(CASE
		    WHEN type = 'expense' AND reimb_status = 'reimbursed'
		    THEN CAST(base_amount_cents AS REAL) / 100 ELSE 0 END), 0)
		FROM transactions
		WHERE user_id = ?
		  AND substr(txn_date, 1, 4) = ?
		  AND type IN ('income', 'expense')
		GROUP BY yr, mo
		ORDER BY mo
	`, userID, fmt.Sprintf("%04d", year))
	if err != nil {
		return nil, fmt.Errorf("monthly stats: %w", err)
	}
	defer rows.Close()
	var stats []MonthlyStat
	for rows.Next() {
		var st MonthlyStat
		if err := rows.Scan(&st.Year, &st.Month, &st.Income, &st.Expense, &st.Reimbursed); err != nil {
			return nil, err
		}
		stats = append(stats, st)
	}
	return stats, rows.Err()
}

// ByCategory returns expense totals grouped by category for a user.
func (s *StatsService) ByCategory(ctx context.Context, userID string, dateFrom, dateTo string) ([]CategoryStat, error) {
	q := `
		SELECT category,
		       COALESCE(SUM(CAST(base_amount_cents AS REAL) / 100), 0) AS total,
		       COUNT(*) AS cnt
		FROM transactions
		WHERE type = 'expense' AND user_id = ?`
	args := []any{userID}
	if dateFrom != "" {
		q += " AND txn_date >= ?"
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		q += " AND txn_date <= ?"
		args = append(args, dateTo)
	}
	q += " GROUP BY category ORDER BY total DESC"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("by-category stats: %w", err)
	}
	defer rows.Close()
	var stats []CategoryStat
	for rows.Next() {
		var st CategoryStat
		if err := rows.Scan(&st.Category, &st.Total, &st.Count); err != nil {
			return nil, err
		}
		stats = append(stats, st)
	}
	return stats, rows.Err()
}

// ByProject returns income/expense totals per project for a user.
func (s *StatsService) ByProject(ctx context.Context, userID string) ([]ProjectStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
		  COALESCE(project_id, ''),
		  COALESCE(project, project_id, ''),
		  COALESCE(SUM(CASE WHEN type = 'income'  THEN CAST(base_amount_cents AS REAL) / 100 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN type = 'expense' THEN CAST(base_amount_cents AS REAL) / 100 ELSE 0 END), 0)
		FROM transactions
		WHERE project_id IS NOT NULL AND project_id != '' AND user_id = ?
		  AND type IN ('income', 'expense')
		GROUP BY project_id
		ORDER BY project_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("by-project stats: %w", err)
	}
	defer rows.Close()
	var stats []ProjectStat
	for rows.Next() {
		var st ProjectStat
		if err := rows.Scan(&st.ProjectID, &st.ProjectName, &st.Income, &st.Expense); err != nil {
			return nil, err
		}
		st.Net = st.Income - st.Expense
		stats = append(stats, st)
	}
	return stats, rows.Err()
}

func (s *StatsService) AccountBalanceHistory(
	ctx context.Context,
	userID string,
	mode model.Mode,
	rangeKey string,
	accountID string,
) ([]BalanceHistoryPoint, error) {
	type window struct {
		start time.Time
		end   time.Time
		all   bool
	}

	resolveWindow := func() (window, error) {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		switch rangeKey {
		case "7d":
			return window{start: today.AddDate(0, 0, -6), end: today}, nil
		case "30d", "":
			return window{start: today.AddDate(0, 0, -29), end: today}, nil
		case "90d":
			return window{start: today.AddDate(0, 0, -89), end: today}, nil
		case "1y":
			return window{start: today.AddDate(-1, 0, 1), end: today}, nil
		case "all":
			return window{all: true}, nil
		default:
			return window{}, fmt.Errorf("invalid range")
		}
	}

	buildWhere := func(includeDate bool) (string, []any) {
		where := " WHERE user_id = ? AND mode = ?"
		args := []any{userID, string(mode)}
		if accountID != "" {
			where += " AND account_id = ?"
			args = append(args, accountID)
		}
		if includeDate {
			where += " AND txn_date >= ? AND txn_date <= ?"
		}
		return where, args
	}

	w, err := resolveWindow()
	if err != nil {
		return nil, err
	}

	if w.all {
		where, args := buildWhere(false)
		var startRaw, endRaw sql.NullString
		if err := s.db.QueryRowContext(ctx, `
			SELECT MIN(txn_date), MAX(txn_date)
			FROM transactions`+where, args...).Scan(&startRaw, &endRaw); err != nil {
			return nil, fmt.Errorf("history window: %w", err)
		}
		if !startRaw.Valid || !endRaw.Valid || startRaw.String == "" || endRaw.String == "" {
			return []BalanceHistoryPoint{}, nil
		}
		startAt, err := time.Parse("2006-01-02", startRaw.String)
		if err != nil {
			return nil, fmt.Errorf("parse start date: %w", err)
		}
		endAt, err := time.Parse("2006-01-02", endRaw.String)
		if err != nil {
			return nil, fmt.Errorf("parse end date: %w", err)
		}
		w.start = startAt
		w.end = endAt
	}

	whereByDate, dateArgs := buildWhere(true)
	startDate := w.start.Format("2006-01-02")
	endDate := w.end.Format("2006-01-02")
	args := append(dateArgs, startDate, endDate)

	rows, err := s.db.QueryContext(ctx, `
		SELECT txn_date,
		  COALESCE(SUM(CASE direction
		    WHEN 'credit' THEN base_amount_cents
		    ELSE -base_amount_cents
		  END), 0) AS delta_cents
		FROM transactions`+whereByDate+`
		GROUP BY txn_date
		ORDER BY txn_date ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("daily deltas: %w", err)
	}
	defer rows.Close()

	deltaByDate := make(map[string]int64)
	for rows.Next() {
		var day string
		var deltaCents int64
		if err := rows.Scan(&day, &deltaCents); err != nil {
			return nil, err
		}
		deltaByDate[day] = deltaCents
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var runningCents int64
	if !w.all {
		where, openingArgs := buildWhere(false)
		openingArgs = append(openingArgs, startDate)
		if err := s.db.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(CASE direction
			  WHEN 'credit' THEN base_amount_cents
			  ELSE -base_amount_cents
			END), 0)
			FROM transactions`+where+` AND txn_date < ?`, openingArgs...).Scan(&runningCents); err != nil {
			return nil, fmt.Errorf("opening balance: %w", err)
		}
	}

	points := make([]BalanceHistoryPoint, 0, int(w.end.Sub(w.start).Hours()/24)+1)
	for d := w.start; !d.After(w.end); d = d.AddDate(0, 0, 1) {
		day := d.Format("2006-01-02")
		if delta, ok := deltaByDate[day]; ok {
			runningCents += delta
		}
		points = append(points, BalanceHistoryPoint{
			Date:    day,
			Balance: float64(runningCents) / 100.0,
		})
	}

	return points, nil
}
