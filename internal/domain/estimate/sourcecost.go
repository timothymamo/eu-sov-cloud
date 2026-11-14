package estimate

import (
	"slices"
	"strings"

	"github.com/timothymamo/shift2eu/internal/domain/inventory"
)

// CurrencyCost is the source's monthly cost in one currency.
type CurrencyCost struct {
	// Currency is the ISO 4217 code, as the inventory gives it.
	Currency string
	// Monthly is the exact sum of the amounts in Currency.
	Monthly Money
	// Resources is how many resources have a cost in Currency.
	Resources int
}

// SourceCost sums what the source charges for some resources (ADR 0028).
type SourceCost struct {
	// Currencies holds one sum per currency, by Currency in byte order, and
	// is nil when no resource has a cost.
	Currencies []CurrencyCost
	// Covered is how many resources have a cost (the sum of every
	// CurrencyCost.Resources).
	Covered int
	// Resources is how many resources there are, with a cost or not: a report
	// shows "for Covered of Resources resources".
	Resources int
}

// SumSourceCost sums the MonthlyCost of resources per currency, never
// converting one currency into another. Each amount is read as the exact
// decimal it prints as, rounded to six fractional digits half away from zero,
// and the amounts are summed exactly. A resource without a cost is counted in
// Resources only, never as 0. A cost that inventory.New refuses (a nil, NaN,
// infinite or negative amount, or an empty currency) is read as no cost.
func SumSourceCost(resources []inventory.Resource) SourceCost {
	sc := SourceCost{Resources: len(resources)}

	for _, r := range resources {
		c := r.MonthlyCost
		if c == nil || c.Currency == "" || c.Amount == nil {
			continue
		}

		amount, ok := exact(*c.Amount)
		if !ok {
			continue
		}

		i, found := slices.BinarySearchFunc(sc.Currencies, c.Currency, func(cc CurrencyCost, code string) int {
			return strings.Compare(cc.Currency, code)
		})
		if !found {
			sc.Currencies = slices.Insert(sc.Currencies, i, CurrencyCost{Currency: c.Currency})
		}

		sc.Currencies[i].Monthly = sc.Currencies[i].Monthly.Add(fromRat(amount))
		sc.Currencies[i].Resources++
		sc.Covered++
	}

	return sc
}
