package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"finarch/internal/domain/model"
)

func TestFindBestMatchesCentsUsesBaseAmounts(t *testing.T) {
	now := time.Now()
	items := []model.Transaction{
		{ID: "a", AmountCents: 10_000, BaseAmountCents: 4_000, OccurredAt: now},
		{ID: "b", AmountCents: 10_000, BaseAmountCents: 6_000, OccurredAt: now},
	}
	results := FindBestMatchesCents(items, 10_000, 0, 2, 5)
	if len(results) == 0 {
		t.Fatal("expected an exact base-currency match")
	}
	if results[0].TotalCents != 10_000 || results[0].ItemCount != 2 {
		t.Fatalf("matching used original cents instead of base cents: %#v", results[0])
	}
}

func TestFindBestMatchesCentsContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := findBestMatchesCentsContext(ctx, []model.Transaction{{
		ID: "a", BaseAmountCents: 100,
	}}, 100, 0, 1, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestFindBestMatchesCentsBoundsReturnedCandidates(t *testing.T) {
	items := make([]model.Transaction, 600)
	for i := range items {
		items[i] = model.Transaction{
			ID:              fmt.Sprintf("tx-%03d", i),
			BaseAmountCents: int64(i + 1),
			OccurredAt:      time.Now(),
		}
	}
	results := FindBestMatchesCents(items, 10_000, 1_000, MaxAllowedDepth, 5)
	if len(results) > 5 {
		t.Fatalf("result limit exceeded: %d", len(results))
	}
}

func TestValidateMatchingAmountsRejectsExcessiveTolerance(t *testing.T) {
	if _, _, err := validateMatchingAmounts(100, 101); err == nil {
		t.Fatal("expected excessive tolerance to be rejected")
	}
}
