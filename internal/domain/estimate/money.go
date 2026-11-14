package estimate

import (
	"math/big"

	"github.com/timothymamo/shift2eu/internal/domain/catalog"
)

// Money is an exact, non-negative amount of euros, held as a rational number so
// that no estimate is ever rounded before it is shown and no product of capped
// inputs can overflow (ADR 0027). The zero Money is 0 EUR.
//
// A Money is immutable: every method returns a new value and never changes its
// receiver or argument, so Money values may be copied and shared freely. Compare
// two values with Equal or Cmp, never with ==; go-cmp uses Equal.
type Money struct {
	// r is the amount in euros; nil is 0. It is never changed once set.
	r *big.Rat
}

// FromAmount returns the catalog amount a as Money: a / catalog.AmountScale
// euros, so FromAmount(125000) is 12.5 EUR exactly.
func FromAmount(a catalog.Amount) Money {
	// TODO(PRD #190 M6): not implemented yet.
	_ = a

	return Money{}
}

// Ratio returns num / den euros exactly, for tests and constants: Ratio(25, 2)
// is 12.5 EUR, Ratio(1, 3) a third of a euro. den must be greater than 0 and num
// at least 0; otherwise Ratio returns the zero Money.
func Ratio(num, den int64) Money {
	// TODO(PRD #190 M6): not implemented yet.
	_, _ = num, den

	return Money{}
}

// Add returns m + n exactly.
func (m Money) Add(n Money) Money {
	// TODO(PRD #190 M6): not implemented yet.
	_ = n

	return Money{}
}

// Cmp returns -1, 0 or +1 as m is less than, equal to or greater than n.
func (m Money) Cmp(n Money) int {
	// TODO(PRD #190 M6): not implemented yet.
	_ = n

	return 0
}

// Equal reports whether m and n are the same amount, exactly. go-cmp calls it,
// so cmp.Diff compares Money by value.
func (m Money) Equal(n Money) bool {
	// TODO(PRD #190 M6): not implemented yet.
	_ = n

	return false
}

// IsZero reports whether m is 0 EUR.
func (m Money) IsZero() bool {
	// TODO(PRD #190 M6): not implemented yet.
	_ = m.r

	return false
}

// String formats m rounded to the cent, half away from zero (ADR 0024), as a
// plain decimal with exactly two fractional digits and no thousands separator:
// Ratio(25, 2) is "12.50", Ratio(1, 200) (half a cent) is "0.01", Ratio(1, 3) is
// "0.33", Ratio(0, 1) is "0.00", Ratio(123456789, 100) is "1234567.89". It is
// the only place an estimate is rounded; a report adds its own currency sign and
// separators.
func (m Money) String() string {
	// TODO(PRD #190 M6): not implemented yet.
	return ""
}

// Exact formats m as an exact fraction in lowest terms, "num/den", or "num"
// when it is whole: Ratio(25, 2) is "25/2", Ratio(4, 2) is "2", the zero Money
// is "0". It is for tests and logs.
func (m Money) Exact() string {
	// TODO(PRD #190 M6): not implemented yet.
	return ""
}
