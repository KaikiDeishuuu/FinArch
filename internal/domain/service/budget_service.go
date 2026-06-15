package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"

	"github.com/google/uuid"
)

// BudgetService manages monthly spending budgets.
type BudgetService struct {
	budgets repository.BudgetRepository
}

// NewBudgetService creates a BudgetService.
func NewBudgetService(budgets repository.BudgetRepository) *BudgetService {
	return &BudgetService{budgets: budgets}
}

type CreateBudgetRequest struct {
	UserID          string
	Mode            model.Mode
	PeriodMonth     string
	Category        string
	AmountCents     int64
	Currency        string
	BaseCurrency    string
	BaseAmountCents int64
}

type UpdateBudgetRequest struct {
	ID              string
	UserID          string
	Mode            model.Mode
	PeriodMonth     string
	Category        string
	AmountCents     int64
	Currency        string
	BaseCurrency    string
	BaseAmountCents int64
}

type BudgetProgress struct {
	Budget         model.Budget
	ActualCents    int64
	RemainingCents int64
	UsageRatio     float64
	Status         string
}

type BudgetSummary struct {
	Mode             model.Mode
	PeriodMonth      string
	TotalActualCents int64
	TotalBudget      *BudgetProgress
	CategoryBudgets  []BudgetProgress
}

func (s *BudgetService) ListBudgets(ctx context.Context, userID string, mode model.Mode, periodMonth string) ([]model.Budget, error) {
	mode, err := normalizeBudgetMode(mode)
	if err != nil {
		return nil, err
	}
	periodMonth, err = normalizePeriodMonth(periodMonth)
	if err != nil {
		return nil, err
	}
	return s.budgets.ListByUserMonth(ctx, userID, mode, periodMonth)
}

func (s *BudgetService) CreateBudget(ctx context.Context, req CreateBudgetRequest) (model.Budget, error) {
	b, err := normalizeBudget(model.Budget{
		ID:              uuid.NewString(),
		UserID:          req.UserID,
		Mode:            req.Mode,
		PeriodMonth:     req.PeriodMonth,
		Category:        req.Category,
		AmountCents:     req.AmountCents,
		Currency:        req.Currency,
		BaseCurrency:    req.BaseCurrency,
		BaseAmountCents: req.BaseAmountCents,
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	})
	if err != nil {
		return model.Budget{}, err
	}
	if err := s.budgets.Create(ctx, b); err != nil {
		return model.Budget{}, fmt.Errorf("预算保存失败，请检查是否已存在相同预算")
	}
	return b, nil
}

func (s *BudgetService) UpdateBudget(ctx context.Context, req UpdateBudgetRequest) (model.Budget, error) {
	if strings.TrimSpace(req.ID) == "" {
		return model.Budget{}, fmt.Errorf("预算不存在")
	}
	existing, err := s.budgets.GetByID(ctx, req.ID, req.UserID)
	if err != nil {
		return model.Budget{}, fmt.Errorf("预算不存在")
	}
	mode := req.Mode
	if mode == "" {
		mode = existing.Mode
	}
	periodMonth := req.PeriodMonth
	if strings.TrimSpace(periodMonth) == "" {
		periodMonth = existing.PeriodMonth
	}
	amountCents := req.AmountCents
	if amountCents <= 0 {
		amountCents = existing.AmountCents
	}
	currency := req.Currency
	if strings.TrimSpace(currency) == "" {
		currency = existing.Currency
	}
	baseCurrency := req.BaseCurrency
	if strings.TrimSpace(baseCurrency) == "" {
		baseCurrency = existing.BaseCurrency
	}
	baseAmountCents := req.BaseAmountCents
	if baseAmountCents <= 0 {
		baseAmountCents = amountCents
	}
	b, err := normalizeBudget(model.Budget{
		ID:              existing.ID,
		UserID:          existing.UserID,
		Mode:            mode,
		PeriodMonth:     periodMonth,
		Category:        req.Category,
		AmountCents:     amountCents,
		Currency:        currency,
		BaseCurrency:    baseCurrency,
		BaseAmountCents: baseAmountCents,
		IsActive:        true,
		CreatedAt:       existing.CreatedAt,
		UpdatedAt:       time.Now(),
	})
	if err != nil {
		return model.Budget{}, err
	}
	if err := s.budgets.Update(ctx, b); err != nil {
		return model.Budget{}, fmt.Errorf("预算更新失败，请检查是否已存在相同预算")
	}
	return b, nil
}

func (s *BudgetService) DeleteBudget(ctx context.Context, userID, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("预算不存在")
	}
	if err := s.budgets.Delete(ctx, id, userID); err != nil {
		return fmt.Errorf("预算不存在")
	}
	return nil
}

func (s *BudgetService) Summary(ctx context.Context, userID string, mode model.Mode, periodMonth string) (BudgetSummary, error) {
	mode, err := normalizeBudgetMode(mode)
	if err != nil {
		return BudgetSummary{}, err
	}
	periodMonth, err = normalizePeriodMonth(periodMonth)
	if err != nil {
		return BudgetSummary{}, err
	}
	budgets, err := s.budgets.ListByUserMonth(ctx, userID, mode, periodMonth)
	if err != nil {
		return BudgetSummary{}, err
	}
	actuals, err := s.budgets.GetMonthlyExpenseActuals(ctx, userID, mode, periodMonth)
	if err != nil {
		return BudgetSummary{}, err
	}
	out := BudgetSummary{
		Mode:             mode,
		PeriodMonth:      periodMonth,
		TotalActualCents: actuals.TotalCents,
		CategoryBudgets:  []BudgetProgress{},
	}
	for _, b := range budgets {
		actual := actuals.TotalCents
		if b.Category != "" {
			actual = actuals.ByCategory[b.Category]
		}
		progress := makeBudgetProgress(b, actual)
		if b.Category == "" {
			p := progress
			out.TotalBudget = &p
		} else {
			out.CategoryBudgets = append(out.CategoryBudgets, progress)
		}
	}
	return out, nil
}

func normalizeBudget(b model.Budget) (model.Budget, error) {
	if strings.TrimSpace(b.UserID) == "" {
		return model.Budget{}, fmt.Errorf("用户不存在")
	}
	mode, err := normalizeBudgetMode(b.Mode)
	if err != nil {
		return model.Budget{}, err
	}
	periodMonth, err := normalizePeriodMonth(b.PeriodMonth)
	if err != nil {
		return model.Budget{}, err
	}
	b.Mode = mode
	b.PeriodMonth = periodMonth
	b.Category = strings.TrimSpace(b.Category)
	if b.AmountCents <= 0 {
		return model.Budget{}, fmt.Errorf("预算金额必须为正数")
	}
	b.Currency = strings.ToUpper(strings.TrimSpace(b.Currency))
	if b.Currency == "" {
		b.Currency = "CNY"
	}
	b.BaseCurrency = strings.ToUpper(strings.TrimSpace(b.BaseCurrency))
	if b.BaseCurrency == "" {
		b.BaseCurrency = b.Currency
	}
	if b.BaseAmountCents <= 0 {
		b.BaseAmountCents = b.AmountCents
	}
	return b, nil
}

func normalizeBudgetMode(mode model.Mode) (model.Mode, error) {
	if mode == "" {
		return model.ModeWork, nil
	}
	if mode != model.ModeWork && mode != model.ModeLife {
		return "", fmt.Errorf("无效的模式")
	}
	return mode, nil
}

func normalizePeriodMonth(periodMonth string) (string, error) {
	periodMonth = strings.TrimSpace(periodMonth)
	if periodMonth == "" {
		return time.Now().Format("2006-01"), nil
	}
	if len(periodMonth) != len("2006-01") {
		return "", fmt.Errorf("预算月份格式必须为 YYYY-MM")
	}
	if _, err := time.Parse("2006-01", periodMonth); err != nil {
		return "", fmt.Errorf("预算月份格式必须为 YYYY-MM")
	}
	return periodMonth, nil
}

func makeBudgetProgress(b model.Budget, actualCents int64) BudgetProgress {
	remaining := b.BaseAmountCents - actualCents
	usage := 0.0
	if b.BaseAmountCents > 0 {
		usage = float64(actualCents) / float64(b.BaseAmountCents)
	}
	status := "ok"
	if usage >= 1 {
		status = "over"
	} else if usage >= 0.8 {
		status = "warning"
	}
	return BudgetProgress{
		Budget:         b,
		ActualCents:    actualCents,
		RemainingCents: remaining,
		UsageRatio:     usage,
		Status:         status,
	}
}
