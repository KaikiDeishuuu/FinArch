package service

import (
	"context"
	"testing"

	"finarch/internal/domain/model"
)

type fakeBudgetRepo struct {
	created model.Budget
	items   []model.Budget
	actuals model.BudgetActuals
}

func (f *fakeBudgetRepo) Create(_ context.Context, b model.Budget) error {
	f.created = b
	return nil
}
func (f *fakeBudgetRepo) GetByID(_ context.Context, id, userID string) (model.Budget, error) {
	for _, b := range f.items {
		if b.ID == id && b.UserID == userID {
			return b, nil
		}
	}
	return model.Budget{}, errBudgetNotFoundForTest{}
}
func (f *fakeBudgetRepo) ListByUserMonth(_ context.Context, _ string, _ model.Mode, _ string) ([]model.Budget, error) {
	return f.items, nil
}
func (f *fakeBudgetRepo) Update(_ context.Context, b model.Budget) error {
	f.created = b
	return nil
}
func (f *fakeBudgetRepo) Delete(context.Context, string, string) error { return nil }
func (f *fakeBudgetRepo) GetMonthlyExpenseActuals(context.Context, string, model.Mode, string) (model.BudgetActuals, error) {
	return f.actuals, nil
}

type errBudgetNotFoundForTest struct{}

func (errBudgetNotFoundForTest) Error() string { return "not found" }

func TestBudgetServiceCreateNormalizesDefaults(t *testing.T) {
	repo := &fakeBudgetRepo{}
	svc := NewBudgetService(repo)
	budget, err := svc.CreateBudget(context.Background(), CreateBudgetRequest{
		UserID:      "u1",
		Mode:        model.ModeLife,
		PeriodMonth: "2026-06",
		Category:    " 餐饮 ",
		AmountCents: 12345,
	})
	if err != nil {
		t.Fatal(err)
	}
	if budget.ID == "" || repo.created.ID == "" {
		t.Fatal("expected generated id")
	}
	if budget.Category != "餐饮" {
		t.Fatalf("category not trimmed: %q", budget.Category)
	}
	if budget.Currency != "CNY" || budget.BaseCurrency != "CNY" {
		t.Fatalf("unexpected currencies: %s/%s", budget.Currency, budget.BaseCurrency)
	}
	if budget.BaseAmountCents != 12345 {
		t.Fatalf("base amount = %d", budget.BaseAmountCents)
	}
}

func TestBudgetServiceRejectsInvalidInputs(t *testing.T) {
	svc := NewBudgetService(&fakeBudgetRepo{})
	cases := []CreateBudgetRequest{
		{UserID: "", Mode: model.ModeWork, PeriodMonth: "2026-06", AmountCents: 1},
		{UserID: "u1", Mode: model.Mode("bad"), PeriodMonth: "2026-06", AmountCents: 1},
		{UserID: "u1", Mode: model.ModeWork, PeriodMonth: "202606", AmountCents: 1},
		{UserID: "u1", Mode: model.ModeWork, PeriodMonth: "2026-06", AmountCents: 0},
	}
	for i, tc := range cases {
		if _, err := svc.CreateBudget(context.Background(), tc); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestBudgetSummaryComputesProgress(t *testing.T) {
	repo := &fakeBudgetRepo{
		items: []model.Budget{
			{ID: "total", UserID: "u1", Mode: model.ModeLife, PeriodMonth: "2026-06", BaseAmountCents: 10000, AmountCents: 10000, Currency: "CNY", BaseCurrency: "CNY", IsActive: true},
			{ID: "food", UserID: "u1", Mode: model.ModeLife, PeriodMonth: "2026-06", Category: "餐饮", BaseAmountCents: 3000, AmountCents: 3000, Currency: "CNY", BaseCurrency: "CNY", IsActive: true},
		},
		actuals: model.BudgetActuals{TotalCents: 8500, ByCategory: map[string]int64{"餐饮": 3200}},
	}
	svc := NewBudgetService(repo)
	summary, err := svc.Summary(context.Background(), "u1", model.ModeLife, "2026-06")
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalBudget == nil || summary.TotalBudget.Status != "warning" {
		t.Fatalf("total status = %#v", summary.TotalBudget)
	}
	if got := summary.TotalBudget.RemainingCents; got != 1500 {
		t.Fatalf("remaining = %d", got)
	}
	if len(summary.CategoryBudgets) != 1 || summary.CategoryBudgets[0].Status != "over" {
		t.Fatalf("category progress = %#v", summary.CategoryBudgets)
	}
}
