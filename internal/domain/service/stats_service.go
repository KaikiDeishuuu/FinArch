package service

import (
	"context"
	"database/sql"
	"fmt"
)

// StatsService provides aggregated financial statistics.
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

// Summary returns company balance + personal outstanding for a user.
func (s *StatsService) Summary(ctx context.Context, userID string) (PoolBalance, error) {
	var b PoolBalance
	err := s.db.QueryRowContext(ctx, `
		SELECT
		  COALESCE(SUM(CASE
		    WHEN source='company' AND direction='income'  THEN  amount_yuan
		    WHEN source='company' AND direction='expense' THEN -amount_yuan
		    ELSE 0 END), 0),
		  COALESCE(SUM(CASE
		    WHEN source='personal' AND direction='expense' AND reimbursed=0
		    THEN amount_yuan ELSE 0 END), 0)
		FROM transactions
		WHERE user_id = ?
	`, userID).Scan(&b.CompanyBalance, &b.PersonalOutstanding)
	if err != nil {
		return PoolBalance{}, fmt.Errorf("summary: %w", err)
	}
	return b, nil
}

// Monthly returns income/expense grouped by month for a given year for a user.
func (s *StatsService) Monthly(ctx context.Context, userID string, year int) ([]MonthlyStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
		  CAST(strftime('%Y', datetime(occurred_at, 'unixepoch')) AS INTEGER),
		  CAST(strftime('%m', datetime(occurred_at, 'unixepoch')) AS INTEGER),
		  COALESCE(SUM(CASE WHEN direction='income' THEN amount_yuan ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN direction='expense' THEN amount_yuan ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN direction='expense' AND source='personal' AND reimbursed=1 THEN amount_yuan ELSE 0 END), 0)
		FROM transactions
		WHERE strftime('%Y', datetime(occurred_at, 'unixepoch')) = ? AND user_id = ?
		GROUP BY 1, 2
		ORDER BY 2
	`, fmt.Sprintf("%d", year), userID)
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
		       COALESCE(SUM(amount_yuan), 0) AS total,
		       COUNT(*) AS cnt
		FROM transactions
		WHERE direction='expense' AND user_id = ?`
	args := []any{userID}
	if dateFrom != "" {
		q += " AND date(datetime(occurred_at,'unixepoch')) >= ?"
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		q += " AND date(datetime(occurred_at,'unixepoch')) <= ?"
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
		SELECT project_id,
		       COALESCE((SELECT name FROM projects WHERE id = project_id), project_id),
		       COALESCE(SUM(CASE WHEN direction='income'  THEN amount_yuan ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN direction='expense' THEN amount_yuan ELSE 0 END), 0)
		FROM transactions
		WHERE project_id IS NOT NULL AND project_id != '' AND user_id = ?
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
