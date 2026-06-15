package repository

import (
	"context"

	"finarch/internal/domain/model"
)

// RecurringTransactionRepository stores recurring rules and generation history.
type RecurringTransactionRepository interface {
	CreateRule(ctx context.Context, rule model.RecurringTransactionRule) error
	GetRuleByID(ctx context.Context, id, userID string) (model.RecurringTransactionRule, error)
	ListRulesByUser(ctx context.Context, userID string, mode model.Mode) ([]model.RecurringTransactionRule, error)
	UpdateRule(ctx context.Context, rule model.RecurringTransactionRule) error
	DeleteRule(ctx context.Context, id, userID string) error
	ListDueRules(ctx context.Context, nowUnix int64, limit int) ([]model.RecurringTransactionRule, error)
	ClaimInstance(ctx context.Context, inst model.RecurringTransactionInstance) (bool, error)
	MarkInstanceGenerated(ctx context.Context, id, transactionID string) error
	MarkInstanceFailed(ctx context.Context, id, message string) error
	ListInstances(ctx context.Context, ruleID, userID string, limit int) ([]model.RecurringTransactionInstance, error)
}
