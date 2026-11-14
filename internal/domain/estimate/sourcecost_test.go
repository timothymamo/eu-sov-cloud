package estimate_test

import (
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/internal/domain/estimate"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
)

// monthly is an invented monthly cost, as an inventory holds what the source
// charges for a resource.
func monthly(amount float64, currency string) *inventory.Cost {
	return &inventory.Cost{Amount: f(amount), Currency: currency}
}

// TestSumSourceCost pins the source's own cost (PRD #190, ADR 0028): one exact
// sum per currency, in currency code order, never converted, with the coverage a
// report shows as "for Covered of Resources resources".
func TestSumSourceCost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		resources []inventory.Resource
		want      estimate.SourceCost
	}{
		{
			name:      "no resources at all",
			resources: nil,
			want:      estimate.SourceCost{},
		},
		{
			name: "no resource has a cost",
			resources: []inventory.Resource{
				{ID: "web-1"},
				{ID: "web-2"},
			},
			want: estimate.SourceCost{Resources: 2},
		},
		{
			name: "one cost in one currency",
			resources: []inventory.Resource{
				{ID: "web-1", MonthlyCost: monthly(12.5, "EUR")},
			},
			want: estimate.SourceCost{
				Currencies: []estimate.CurrencyCost{
					{Currency: "EUR", Monthly: estimate.Ratio(25, 2), Resources: 1},
				},
				Covered: 1, Resources: 1,
			},
		},
		{
			// Two entries, one per currency, sorted by code: the amounts are
			// never combined into one number.
			name: "one sum per currency, in currency order, never converted",
			resources: []inventory.Resource{
				{ID: "web-1", MonthlyCost: monthly(3, "USD")},
				{ID: "web-2", MonthlyCost: monthly(12.5, "EUR")},
				{ID: "web-3", MonthlyCost: monthly(4, "USD")},
				{ID: "web-4", MonthlyCost: monthly(25, "EUR")},
			},
			want: estimate.SourceCost{
				Currencies: []estimate.CurrencyCost{
					{Currency: "EUR", Monthly: estimate.Ratio(75, 2), Resources: 2},
					{Currency: "USD", Monthly: estimate.Ratio(7, 1), Resources: 2},
				},
				Covered: 4, Resources: 4,
			},
		},
		{
			// 0.1 + 0.2 is exactly 3/10, not the float sum 0.30000000000000004:
			// the sum is exact and rounded only by Money.String, to the cent.
			name: "the sum is exact, never rounded before it is shown",
			resources: []inventory.Resource{
				{ID: "web-1", MonthlyCost: monthly(0.1, "EUR")},
				{ID: "web-2", MonthlyCost: monthly(0.2, "EUR")},
			},
			want: estimate.SourceCost{
				Currencies: []estimate.CurrencyCost{
					{Currency: "EUR", Monthly: estimate.Ratio(3, 10), Resources: 2},
				},
				Covered: 2, Resources: 2,
			},
		},
		{
			// Each amount is read to six fractional digits, half away from zero,
			// before it is summed: the seventh digit 5 rounds the sixth up.
			name: "an amount is read to six fractional digits, half away from zero",
			resources: []inventory.Resource{
				{ID: "web-1", MonthlyCost: monthly(0.1234565, "EUR")},
			},
			want: estimate.SourceCost{
				Currencies: []estimate.CurrencyCost{
					{Currency: "EUR", Monthly: estimate.Ratio(123457, 1000000), Resources: 1},
				},
				Covered: 1, Resources: 1,
			},
		},
		{
			// Each of the two amounts rounds up to a millionth before the sum,
			// so the total is two millionths and not one.
			name: "every amount is rounded before the amounts are summed",
			resources: []inventory.Resource{
				{ID: "web-1", MonthlyCost: monthly(0.0000005, "EUR")},
				{ID: "web-2", MonthlyCost: monthly(0.0000005, "EUR")},
			},
			want: estimate.SourceCost{
				Currencies: []estimate.CurrencyCost{
					{Currency: "EUR", Monthly: estimate.Ratio(1, 500000), Resources: 2},
				},
				Covered: 2, Resources: 2,
			},
		},
		{
			// A free tier is a cost of 0, so the resource is covered.
			name: "a free resource is covered",
			resources: []inventory.Resource{
				{ID: "web-1", MonthlyCost: monthly(0, "EUR")},
			},
			want: estimate.SourceCost{
				Currencies: []estimate.CurrencyCost{{Currency: "EUR", Resources: 1}},
				Covered:    1, Resources: 1,
			},
		},
		{
			// A resource without a cost is counted in Resources, never added as
			// 0 EUR and never covered.
			name: "a resource without a cost is not counted as 0",
			resources: []inventory.Resource{
				{ID: "web-1"},
				{ID: "web-2", MonthlyCost: monthly(0, "EUR")},
			},
			want: estimate.SourceCost{
				Currencies: []estimate.CurrencyCost{{Currency: "EUR", Resources: 1}},
				Covered:    1, Resources: 2,
			},
		},
		{
			// Only an inventory built without inventory.New can hold these: a
			// cost that is not a cost is read as no cost.
			name: "a cost without an amount, or that is not a cost, is no cost",
			resources: []inventory.Resource{
				{ID: "web-1", MonthlyCost: &inventory.Cost{Currency: "EUR"}},
				{ID: "web-2", MonthlyCost: &inventory.Cost{Amount: f(math.NaN()), Currency: "EUR"}},
				{ID: "web-3", MonthlyCost: &inventory.Cost{Amount: f(math.Inf(1)), Currency: "EUR"}},
				{ID: "web-4", MonthlyCost: &inventory.Cost{Amount: f(math.Inf(-1)), Currency: "EUR"}},
				{ID: "web-5", MonthlyCost: &inventory.Cost{Amount: f(-1), Currency: "EUR"}},
				{ID: "web-6", MonthlyCost: &inventory.Cost{Amount: f(5), Currency: ""}},
			},
			want: estimate.SourceCost{Resources: 6},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := estimate.SumSourceCost(tc.resources)

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("SumSourceCost() mismatch (-want +got):\n%s", diff)
			}

			// Covered is the sum of the per-currency resource counts, and every
			// sum is in its currency.
			covered := 0

			for _, c := range got.Currencies {
				covered += c.Resources

				if c.Currency == "" {
					t.Errorf("Currencies holds a sum without a currency: %+v", c)
				}
			}

			if covered != got.Covered {
				t.Errorf("Currencies cover %d resources, want Covered = %d", covered, got.Covered)
			}
		})
	}
}

// TestSumSourceCostIsExact pins that two sums that round alike as Money.String
// are still distinct values: the amount is exact, and a report rounds it only
// when it prints it.
func TestSumSourceCostIsExact(t *testing.T) {
	t.Parallel()

	// Both amounts print "0.00": a tenth of a cent and a fifth of a cent.
	one := estimate.SumSourceCost([]inventory.Resource{
		{ID: "web-1", MonthlyCost: monthly(0.001, "EUR")},
	})
	two := estimate.SumSourceCost([]inventory.Resource{
		{ID: "web-1", MonthlyCost: monthly(0.002, "EUR")},
	})

	if len(one.Currencies) != 1 || len(two.Currencies) != 1 {
		t.Fatalf("Currencies = %+v and %+v, want one sum each", one.Currencies, two.Currencies)
	}

	first, second := one.Currencies[0].Monthly, two.Currencies[0].Monthly

	if first.String() != "0.00" || second.String() != "0.00" {
		t.Fatalf("the printed amounts are %q and %q, want both rounded to \"0.00\"",
			first.String(), second.String())
	}

	// Each amount is kept to six fractional digits, so 0.001 is a thousandth
	// and 0.002 a five-hundredth: the two sums are distinct values, whatever
	// their printed form.
	if first.Equal(second) {
		t.Errorf("%s and %s sum to the same Money, want them distinct", first.Exact(), second.Exact())
	}

	if diff := cmp.Diff(estimate.Ratio(1, 1000), first); diff != "" {
		t.Errorf("0.001 EUR is not exactly 1/1000 (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(estimate.Ratio(1, 500), second); diff != "" {
		t.Errorf("0.002 EUR is not exactly 1/500 (-want +got):\n%s", diff)
	}
}
