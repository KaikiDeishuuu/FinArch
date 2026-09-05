package model

import (
	"errors"
	"math"
	"testing"
)

func TestMoneyCentsRoundsAndPreservesSign(t *testing.T) {
	tests := []struct {
		name string
		yuan Money
		want int64
	}{
		{name: "binary floating point regression", yuan: 0.29, want: 29},
		{name: "round up", yuan: 0.006, want: 1},
		{name: "positive decimal half", yuan: 1.005, want: 101},
		{name: "small decimal half", yuan: 0.145, want: 15},
		{name: "negative decimal half", yuan: -1.005, want: -101},
		{name: "negative", yuan: -0.29, want: -29},
		{name: "zero", yuan: 0, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.yuan.Cents()
			if err != nil {
				t.Fatalf("Cents() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Cents() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMoneyCentsRejectsNonFiniteAndOutOfRange(t *testing.T) {
	tests := []struct {
		name string
		yuan Money
		want error
	}{
		{name: "nan", yuan: Money(math.NaN()), want: ErrInvalidMoney},
		{name: "positive infinity", yuan: Money(math.Inf(1)), want: ErrInvalidMoney},
		{name: "negative infinity", yuan: Money(math.Inf(-1)), want: ErrInvalidMoney},
		{name: "positive overflow", yuan: Money(math.MaxFloat64), want: ErrMoneyOutOfRange},
		{name: "negative overflow", yuan: Money(-math.MaxFloat64), want: ErrMoneyOutOfRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.yuan.Cents()
			if !errors.Is(err, tt.want) {
				t.Fatalf("Cents() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestConvertCentsByRateChecksInt64Boundary(t *testing.T) {
	got, err := ConvertCentsByRate(math.MaxInt64, 1)
	if err != nil || got != math.MaxInt64 {
		t.Fatalf("identity boundary = (%d, %v), want (%d, nil)", got, err, int64(math.MaxInt64))
	}
	if _, err := ConvertCentsByRate(math.MaxInt64, 1.0000000001); !errors.Is(err, ErrMoneyOutOfRange) {
		t.Fatalf("overflow error = %v, want ErrMoneyOutOfRange", err)
	}
	for _, rate := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1, 0} {
		if _, err := ConvertCentsByRate(1, rate); err == nil {
			t.Fatalf("rate %v unexpectedly accepted", rate)
		}
	}
}
