package estimate_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/estimate"
)

// Money is exact: it holds a fraction of a euro and is rounded to the cent only
// when it is shown, half away from zero (ADR 0024, ADR 0027). Every test here
// pins that, because an estimate is never rounded before it is displayed and a
// total is never a rounded sum of rounded parts.

// TestMoneyFromAmount pins the catalog amount to euro conversion: an Amount is
// ten-thousandths of a euro.
func TestMoneyFromAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		amount    catalog.Amount
		want      string
		wantExact string
	}{
		{name: "zero", amount: 0, want: "0.00", wantExact: "0"},
		{name: "a whole euro", amount: 10000, want: "1.00", wantExact: "1"},
		{name: "a round price", amount: 125000, want: "12.50", wantExact: "25/2"},
		{name: "a price with four decimals", amount: 130125, want: "13.01", wantExact: "1041/80"},
		{name: "a per-GiB price", amount: 400, want: "0.04", wantExact: "1/25"},
		{name: "the largest amount", amount: catalog.MaxAmount, want: "1000000.00", wantExact: "1000000"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := estimate.FromAmount(tc.amount)

			if got := m.String(); got != tc.want {
				t.Errorf("FromAmount(%s).String() = %q, want %q", tc.amount, got, tc.want)
			}

			if got := m.Exact(); got != tc.wantExact {
				t.Errorf("FromAmount(%s).Exact() = %q, want %q", tc.amount, got, tc.wantExact)
			}
		})
	}
}

// TestMoneyRatio pins Ratio, and the display rounding: half a cent goes away from
// zero, never to the even cent, and the fraction is shown with exactly two
// digits.
func TestMoneyRatio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		num, den  int64
		want      string
		wantExact string
	}{
		{name: "a round fraction", num: 25, den: 2, want: "12.50", wantExact: "25/2"},
		{name: "half a cent", num: 1, den: 200, want: "0.01", wantExact: "1/200"},
		{name: "a third", num: 1, den: 3, want: "0.33", wantExact: "1/3"},
		{name: "two thirds", num: 2, den: 3, want: "0.67", wantExact: "2/3"},
		{name: "zero", num: 0, den: 1, want: "0.00", wantExact: "0"},
		{name: "a large amount", num: 123456789, den: 100, want: "1234567.89", wantExact: "123456789/100"},
		{name: "exactly one euro and half a cent", num: 1005, den: 1000, want: "1.01", wantExact: "201/200"},
		{name: "a whole number in lowest terms", num: 4, den: 2, want: "2.00", wantExact: "2"},
		{name: "one euro", num: 1, den: 1, want: "1.00", wantExact: "1"},
		{name: "seven eighths", num: 7, den: 8, want: "0.88", wantExact: "7/8"},
		{name: "one eighth", num: 1, den: 8, want: "0.13", wantExact: "1/8"},
		{name: "three eighths", num: 3, den: 8, want: "0.38", wantExact: "3/8"},
		{name: "one cent", num: 1, den: 100, want: "0.01", wantExact: "1/100"},
		{name: "nine tenths of a cent", num: 9, den: 1000, want: "0.01", wantExact: "9/1000"},
		{name: "four tenths of a cent", num: 4, den: 1000, want: "0.00", wantExact: "1/250"},
		{name: "five tenths of a cent", num: 5, den: 1000, want: "0.01", wantExact: "1/200"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := estimate.Ratio(tc.num, tc.den)

			if got := m.String(); got != tc.want {
				t.Errorf("Ratio(%d, %d).String() = %q, want %q", tc.num, tc.den, got, tc.want)
			}

			if got := m.Exact(); got != tc.wantExact {
				t.Errorf("Ratio(%d, %d).Exact() = %q, want %q", tc.num, tc.den, got, tc.wantExact)
			}
		})
	}
}

// TestMoneyRatioRefusesImpossibleAmounts pins that Ratio never builds a negative
// amount or divides by zero: the zero Money is the one value it can return.
func TestMoneyRatioRefusesImpossibleAmounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		num, den int64
	}{
		{name: "a negative amount", num: -1, den: 2},
		{name: "a zero denominator", num: 1, den: 0},
		{name: "a negative denominator", num: 1, den: -2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := estimate.Ratio(tc.num, tc.den)

			if !m.IsZero() {
				t.Errorf("Ratio(%d, %d) = %s, want the zero Money", tc.num, tc.den, m.Exact())
			}

			if got, want := m.String(), "0.00"; got != want {
				t.Errorf("Ratio(%d, %d).String() = %q, want %q", tc.num, tc.den, got, want)
			}
		})
	}
}

// TestMoneyArithmetic pins Add, Cmp, Equal and IsZero, that the zero value is
// 0 EUR, and that no operation changes its operands.
func TestMoneyArithmetic(t *testing.T) {
	t.Parallel()

	half := estimate.Ratio(1, 2)
	third := estimate.Ratio(1, 3)
	twoquarters := estimate.Ratio(2, 4)

	t.Run("add", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name   string
			a, b   estimate.Money
			want   string
			wantEx string
		}{
			{name: "two amounts", a: estimate.Ratio(25, 2), b: estimate.Ratio(1, 4), want: "12.75", wantEx: "51/4"},
			{name: "zero and an amount", a: estimate.Money{}, b: half, want: "0.50", wantEx: "1/2"},
			{name: "amounts that do not round", a: third, b: third, want: "0.67", wantEx: "2/3"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				got := tc.a.Add(tc.b)
				if want := tc.want; got.String() != want {
					t.Errorf("Add().String() = %q, want %q", got.String(), want)
				}

				if want := tc.wantEx; got.Exact() != want {
					t.Errorf("Add().Exact() = %q, want %q", got.Exact(), want)
				}

				// Add is commutative, and neither operand changed.
				if diff := cmp.Diff(tc.a.Add(tc.b), tc.b.Add(tc.a)); diff != "" {
					t.Errorf("Add() is not commutative (-a+b +b+a):\n%s", diff)
				}

				if tc.a.Cmp(estimate.Money{}) == 0 && !tc.a.IsZero() {
					t.Errorf("Add() changed its receiver: %s became zero", tc.a.Exact())
				}

				if tc.b.Cmp(estimate.Money{}) == 0 && !tc.b.IsZero() {
					t.Errorf("Add() changed its argument: %s became zero", tc.b.Exact())
				}
			})
		}
	})

	t.Run("compare", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name  string
			a, b  estimate.Money
			want  int
			equal bool
		}{
			{name: "less", a: third, b: half, want: -1, equal: false},
			{name: "greater", a: half, b: third, want: 1, equal: false},
			{name: "the same amount written differently", a: half, b: twoquarters, want: 0, equal: true},
			{name: "zero and zero", a: estimate.Money{}, b: estimate.Ratio(0, 7), want: 0, equal: true},
			{name: "zero and a tiny amount", a: estimate.Money{}, b: estimate.Ratio(1, 1000), want: -1, equal: false},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				if got := tc.a.Cmp(tc.b); got != tc.want {
					t.Errorf("Cmp() = %d, want %d", got, tc.want)
				}

				if got := tc.a.Equal(tc.b); got != tc.equal {
					t.Errorf("Equal() = %t, want %t", got, tc.equal)
				}
			})
		}
	})

	t.Run("is zero", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			in   estimate.Money
			want bool
		}{
			{name: "the zero value", in: estimate.Money{}, want: true},
			{name: "a ratio of zero", in: estimate.Ratio(0, 5), want: true},
			{name: "an amount below half a cent", in: estimate.Ratio(1, 1000), want: false},
			{name: "an amount", in: estimate.FromAmount(1), want: false},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				if got := tc.in.IsZero(); got != tc.want {
					t.Errorf("IsZero() = %t, want %t", got, tc.want)
				}
			})
		}
	})
}

// TestMoneyZeroValue pins that an unset Money is 0 EUR, so a value built without
// a constructor is never an unprintable or negative amount.
func TestMoneyZeroValue(t *testing.T) {
	t.Parallel()

	var m estimate.Money

	if got, want := m.String(), "0.00"; got != want {
		t.Errorf("the zero Money prints as %q, want %q", got, want)
	}

	if got, want := m.Exact(), "0"; got != want {
		t.Errorf("the zero Money is %q, want %q", got, want)
	}

	if got, want := m.Add(estimate.Ratio(1, 2)).String(), "0.50"; got != want {
		t.Errorf("0 + 1/2 prints as %q, want %q", got, want)
	}
}

// TestMoneyIsComparedByValue pins that go-cmp compares two Money values by their
// amount, never by the fraction they are written as: a test comparing estimates
// must not fail because 1/2 and 2/4 are different internals.
func TestMoneyIsComparedByValue(t *testing.T) {
	t.Parallel()

	type priced struct{ Monthly estimate.Money }

	same := []priced{
		{Monthly: estimate.Ratio(1, 2)},
		{Monthly: estimate.Ratio(2, 4)},
	}

	if diff := cmp.Diff(same[0], same[1]); diff != "" {
		t.Errorf("two equal amounts compare as different (-want +got):\n%s", diff)
	}

	different := []priced{
		{Monthly: estimate.Ratio(1, 2)},
		{Monthly: estimate.Ratio(1, 3)},
	}

	if diff := cmp.Diff(different[0], different[1]); diff == "" {
		t.Error("two different amounts compare as equal, want a difference")
	}
}
