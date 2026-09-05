package service

import (
	"context"
	"testing"

	"finarch/internal/domain/model"
)

func TestBudgetUpdatePreservesOmittedCategoryAndBaseAmount(t *testing.T) {
	existing := model.Budget{
		ID: "b1", UserID: "u1", Mode: model.ModeLife, PeriodMonth: "2026-09",
		Category: "餐饮", AmountCents: 1_000, Currency: "USD",
		BaseCurrency: "CNY", BaseAmountCents: 7_000, IsActive: true,
	}
	repo := &fakeBudgetRepo{items: []model.Budget{existing}}
	updated, err := NewBudgetService(repo).UpdateBudget(context.Background(), UpdateBudgetRequest{
		ID: "b1", UserID: "u1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Category != existing.Category || updated.BaseAmountCents != existing.BaseAmountCents {
		t.Fatalf("omitted fields were overwritten: %#v", updated)
	}
}

func TestBudgetUpdateCanClearCategory(t *testing.T) {
	existing := model.Budget{
		ID: "b1", UserID: "u1", Mode: model.ModeLife, PeriodMonth: "2026-09",
		Category: "餐饮", AmountCents: 1_000, Currency: "CNY",
		BaseCurrency: "CNY", BaseAmountCents: 1_000, IsActive: true,
	}
	repo := &fakeBudgetRepo{items: []model.Budget{existing}}
	emptyCategory := ""
	updated, err := NewBudgetService(repo).UpdateBudget(context.Background(), UpdateBudgetRequest{
		ID: "b1", UserID: "u1", Category: &emptyCategory,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Category != "" {
		t.Fatalf("category = %q, want total-budget scope", updated.Category)
	}
}

func TestBudgetUpdateRequiresBaseAmountForCrossCurrencyChange(t *testing.T) {
	existing := model.Budget{
		ID: "b1", UserID: "u1", Mode: model.ModeLife, PeriodMonth: "2026-09",
		Category: "餐饮", AmountCents: 1_000, Currency: "USD",
		BaseCurrency: "CNY", BaseAmountCents: 7_000, IsActive: true,
	}
	repo := &fakeBudgetRepo{items: []model.Budget{existing}}
	_, err := NewBudgetService(repo).UpdateBudget(context.Background(), UpdateBudgetRequest{
		ID: "b1", UserID: "u1", AmountCents: 2_000,
	})
	if err == nil {
		t.Fatal("expected missing cross-currency base amount to be rejected")
	}
}
