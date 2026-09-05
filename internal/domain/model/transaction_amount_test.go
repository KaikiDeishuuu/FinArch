package model

import (
	"errors"
	"testing"
)

func TestValidateTransactionAmountCentsBoundaries(t *testing.T) {
	for _, amount := range []int64{1, MaxTransactionAmountCents} {
		if err := ValidateTransactionAmountCents(amount); err != nil {
			t.Fatalf("amount %d rejected: %v", amount, err)
		}
	}
	for _, amount := range []int64{-1, 0, MaxTransactionAmountCents + 1} {
		if err := ValidateTransactionAmountCents(amount); !errors.Is(err, ErrTransactionAmountOutOfRange) {
			t.Fatalf("amount %d error = %v, want ErrTransactionAmountOutOfRange", amount, err)
		}
	}
}
