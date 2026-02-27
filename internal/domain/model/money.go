package model

import "math"

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
