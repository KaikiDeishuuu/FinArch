package service

import (
	"math"
	"testing"

	"finarch/internal/domain/model"
)

func validRecurringAmountRequest(amount model.Money) UpsertRecurringRuleRequest {
	return UpsertRecurringRuleRequest{
		UserID:     "u1",
		Mode:       model.ModeWork,
		AccountID:  "a1",
		TxType:     model.TxTypeExpense,
		Category:   "recurring",
		AmountYuan: amount,
		Currency:   "CNY",
		Frequency:  model.RecurringFrequencyMonthly,
	}
}

func TestNormalizeRecurringRuleRoundsLegacyYuanToCents(t *testing.T) {
	svc := &RecurringTransactionService{}
	rule, err := svc.normalizeRule(validRecurringAmountRequest(0.29), nil)
	if err != nil {
		t.Fatalf("normalize 0.29 recurring rule: %v", err)
	}
	if rule.AmountCents != 29 {
		t.Fatalf("amount cents = %d, want 29", rule.AmountCents)
	}
}

func TestNormalizeRecurringRuleRejectsInvalidAmountOnUpdate(t *testing.T) {
	svc := &RecurringTransactionService{}
	existing, err := svc.normalizeRule(validRecurringAmountRequest(1), nil)
	if err != nil {
		t.Fatalf("create fixture rule: %v", err)
	}

	invalid := []model.Money{-0.29, model.Money(math.NaN()), model.Money(math.Inf(1)), model.Money(math.MaxFloat64)}
	for _, amount := range invalid {
		req := UpsertRecurringRuleRequest{ID: existing.ID, UserID: existing.UserID, AmountYuan: amount}
		if _, err := svc.normalizeRule(req, &existing); err == nil {
			t.Fatalf("update amount %v unexpectedly retained the old valid amount", amount)
		}
	}
	if _, err := svc.normalizeRule(UpsertRecurringRuleRequest{
		ID: existing.ID, UserID: existing.UserID, AmountCents: -1,
	}, &existing); err == nil {
		t.Fatal("negative amount_cents unexpectedly retained the old valid amount")
	}
	if _, err := svc.normalizeRule(UpsertRecurringRuleRequest{
		ID: existing.ID, UserID: existing.UserID, AmountProvided: true,
	}, &existing); err == nil {
		t.Fatal("explicit zero amount unexpectedly retained the old valid amount")
	}
}
