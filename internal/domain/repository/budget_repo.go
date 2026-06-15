package repository

import (
	"context"

	"finarch/internal/domain/model"
)

// BudgetRepository defines monthly budget persistence behavior.
type BudgetRepository interface {
	Create(ctx context.Context, budget model.Budget) error
	GetByID(ctx context.Context, id, userID string) (model.Budget, error)
	ListByUserMonth(ctx context.Context, userID string, mode model.Mode, periodMonth string) ([]model.Budget, error)
	Update(ctx context.Context, budget model.Budget) error
	Delete(ctx context.Context, id, userID string) error
	GetMonthlyExpenseActuals(ctx context.Context, userID string, mode model.Mode, periodMonth string) (model.BudgetActuals, error)
}
