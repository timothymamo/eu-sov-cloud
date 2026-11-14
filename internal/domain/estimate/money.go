package estimate

import (
	"fmt"
	"math/big"

	"github.com/timothymamo/shift2eu/internal/domain/catalog"
)

// Money is an exact, non-negative amount, held as a rational number so that no
// estimate is ever rounded before it is shown and no product of capped inputs
// can overflow (ADR 0027). Money carries no currency: estimates and totals are
// in euros (ADR 0024), and a CurrencyCost's Monthly is in its Currency
// (ADR 0028). The zero Money is 0.
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
	return Ratio(int64(a), catalog.AmountScale)
}

// Ratio returns num / den euros exactly, for tests and constants: Ratio(25, 2)
// is 12.5 EUR, Ratio(1, 3) a third of a euro. den must be greater than 0 and num
// at least 0; otherwise Ratio returns the zero Money.
func Ratio(num, den int64) Money {
	if den <= 0 || num < 0 {
		return Money{}
	}

	return Money{r: big.NewRat(num, den)}
}

// fromRat returns r as Money, keeping r: the caller must not change it later.
func fromRat(r *big.Rat) Money {
	return Money{r: r}
}

// rat returns m's amount, a fresh value the caller may change.
func (m Money) rat() *big.Rat {
	if m.r == nil {
		return new(big.Rat)
	}

	return new(big.Rat).Set(m.r)
}

// Add returns m + n exactly.
func (m Money) Add(n Money) Money {
	return fromRat(m.rat().Add(m.rat(), n.rat()))
}

// Cmp returns -1, 0 or +1 as m is less than, equal to or greater than n.
func (m Money) Cmp(n Money) int {
	return m.rat().Cmp(n.rat())
}

// Equal reports whether m and n are the same amount, exactly. go-cmp calls it,
// so cmp.Diff compares Money by value.
func (m Money) Equal(n Money) bool {
	return m.Cmp(n) == 0
}

// IsZero reports whether m is 0 EUR.
func (m Money) IsZero() bool {
	return m.r == nil || m.r.Sign() == 0
}

// String formats m rounded to the cent, half away from zero (ADR 0024), as a
// plain decimal with exactly two fractional digits and no thousands separator:
// Ratio(25, 2) is "12.50", Ratio(1, 200) (half a cent) is "0.01", Ratio(1, 3) is
// "0.33", Ratio(0, 1) is "0.00", Ratio(123456789, 100) is "1234567.89". It is
// the only place an estimate is rounded; a report adds its own currency sign and
// separators.
func (m Money) String() string {
	// Money is never negative, so floor(m × 100 + 1/2) rounds half away from
	// zero.
	r := m.rat()
	r.Mul(r, big.NewRat(100, 1))
	r.Add(r, big.NewRat(1, 2))

	cents := new(big.Int).Quo(r.Num(), r.Denom())
	euros, rest := new(big.Int).QuoRem(cents, big.NewInt(100), new(big.Int))

	return fmt.Sprintf("%s.%02d", euros.String(), rest.Int64())
}

// Exact formats m as an exact fraction in lowest terms, "num/den", or "num"
// when it is whole: Ratio(25, 2) is "25/2", Ratio(4, 2) is "2", the zero Money
// is "0". It is for tests and logs.
func (m Money) Exact() string {
	return m.rat().RatString()
}
