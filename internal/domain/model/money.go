package model

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
)

var (
	// ErrInvalidMoney reports a non-finite monetary value.
	ErrInvalidMoney = errors.New("money must be finite")
	// ErrMoneyOutOfRange reports a monetary result that cannot be represented
	// as signed 64-bit cents.
	ErrMoneyOutOfRange = errors.New("money is outside the supported cents range")
	// ErrTransactionAmountOutOfRange reports a transaction amount outside the
	// positive, JSON-safe range accepted by the domain.
	ErrTransactionAmountOutOfRange = errors.New("transaction amount is outside the supported range")
)

// MaxTransactionAmountCents is the largest source or base-currency amount
// accepted for a single transaction. It stays below JavaScript's maximum safe
// integer while leaving ample headroom inside int64 for account aggregation.
const MaxTransactionAmountCents int64 = 9_000_000_000_000_000

// Money represents amount in yuan.
type Money float64

// Float64 returns the underlying value in yuan.
func (m Money) Float64() float64 {
	return float64(m)
}

// Abs returns the absolute value of money.
func (m Money) Abs() Money {
	return Money(math.Abs(float64(m)))
}

// Cents converts a legacy yuan-denominated floating-point amount to integer
// cents. The shortest decimal representation of the input is rounded half
// away from zero, avoiding binary floating-point errors at values such as
// 1.005. Non-finite and out-of-range inputs are rejected.
func (m Money) Cents() (int64, error) {
	yuan := float64(m)
	if math.IsNaN(yuan) || math.IsInf(yuan, 0) {
		return 0, ErrInvalidMoney
	}
	decimal, ok := new(big.Rat).SetString(strconv.FormatFloat(yuan, "g"[0], -1, 64))
	if !ok {
		return 0, ErrMoneyOutOfRange
	}
	return roundRatToInt64(new(big.Rat).Mul(decimal, big.NewRat(100, 1)))
}

// ConvertCentsByRate applies a positive finite exchange rate and rounds to the
// nearest cent without allowing big.Int-to-int64 wraparound.
func ConvertCentsByRate(amountCents int64, rate float64) (int64, error) {
	if amountCents <= 0 || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, fmt.Errorf("invalid amount or rate")
	}
	rateRat, ok := new(big.Rat).SetString(strconv.FormatFloat(rate, "g"[0], -1, 64))
	if !ok || rateRat.Sign() <= 0 {
		return 0, fmt.Errorf("invalid rate")
	}
	result := new(big.Rat).Mul(big.NewRat(amountCents, 1), rateRat)
	return roundRatToInt64(result)
}

// ValidateTransactionAmountCents applies the domain range for both source and
// base-currency transaction amounts.
func ValidateTransactionAmountCents(amountCents int64) error {
	if amountCents <= 0 || amountCents > MaxTransactionAmountCents {
		return ErrTransactionAmountOutOfRange
	}
	return nil
}

func roundRatToInt64(value *big.Rat) (int64, error) {
	if value == nil {
		return 0, ErrMoneyOutOfRange
	}
	numerator := new(big.Int).Set(value.Num())
	negative := numerator.Sign() < 0
	numerator.Abs(numerator)
	denominator := new(big.Int).Set(value.Denom())
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if remainder.Lsh(remainder, 1).Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if negative {
		quotient.Neg(quotient)
	}
	if !quotient.IsInt64() {
		return 0, ErrMoneyOutOfRange
	}
	return quotient.Int64(), nil
}
