// Package testutil_test pins the fixtures other tests share. The priced fixture
// is the one plans and prices come from, so its canonical form, its default
// region and the fact that every call returns fresh slices are pinned here: a
// test that accepted a fixture the domain would never produce would pin the wrong
// behaviour.
package testutil_test

import (
	"slices"
	"testing"

	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/testutil"
)

// TestPricedCatalog pins the priced fixture: it is a catalog the domain accepts,
// it names its default region, every amount is non-negative and in EUR, and its
// plans come back sorted by name with their prices sorted by first region, so a
// test that reads them sees the canonical form.
func TestPricedCatalog(t *testing.T) {
	t.Parallel()

	cat := testutil.PricedCatalog(t)

	if got, want := cat.Target(), "Example Cloud"; got != want {
		t.Errorf("Target() = %q, want %q", got, want)
	}

	if got, want := cat.Snapshot().DefaultRegion, testutil.PricedDefaultRegion; got != want {
		t.Errorf("Snapshot().DefaultRegion = %q, want %q", got, want)
	}

	offerings := cat.Offerings()
	if len(offerings) != 2 {
		t.Fatalf("Offerings() returned %d offerings, want 2", len(offerings))
	}

	// The offerings come back sorted by name, so the compute one is found by
	// name: the tests that read a plan must not depend on where it sits.
	machinesName := "Example Virtual Machines"

	index := slices.IndexFunc(offerings, func(o catalog.Offering) bool {
		return o.Name == machinesName
	})
	if index < 0 {
		t.Fatalf("Offerings() holds no offering named %q", machinesName)
	}

	machines := offerings[index]

	wantPlanNames := []string{"EX-L", "EX-M", "EX-S"}

	gotPlanNames := make([]string, len(machines.Plans))
	for i, plan := range machines.Plans {
		gotPlanNames[i] = plan.Name
	}

	if !slices.Equal(gotPlanNames, wantPlanNames) {
		t.Errorf("plan names = %v, want %v, sorted by name", gotPlanNames, wantPlanNames)
	}

	if len(machines.Plans) != 3 {
		t.Fatalf("the compute offering has %d plans, want 3", len(machines.Plans))
	}

	// Two plans are priced region by region and one has a single every-region
	// price: EX-L comes first, at a different amount for each region, and EX-S
	// last, with the price that applies to every region.
	if len(machines.Plans[0].Prices) != 2 {
		t.Fatalf("EX-L has %d prices, want 2: one for each region",
			len(machines.Plans[0].Prices))
	}

	if got := machines.Plans[0].Prices[0].Regions; !slices.Equal(got, []string{"xx-ams"}) {
		t.Errorf("EX-L's first price regions = %v, want [xx-ams]", got)
	}

	if got := machines.Plans[0].Prices[1].Regions; !slices.Equal(got, []string{"xx-par"}) {
		t.Errorf("EX-L's second price regions = %v, want [xx-par]", got)
	}

	if got, want := machines.Plans[0].Prices[0].Amount, machines.Plans[0].Prices[1].Amount; got == want {
		t.Errorf("EX-L's two prices both amount to %s, want a different amount per region", got)
	}

	// EX-S sells one price that applies to every region, which the catalog keeps
	// as a price with no regions at all (ADR 0024).
	if len(machines.Plans[2].Prices) != 1 {
		t.Fatalf("EX-S has %d prices, want 1", len(machines.Plans[2].Prices))
	}

	if regions := machines.Plans[2].Prices[0].Regions; regions != nil {
		t.Errorf("EX-S's price applies to %v, want every region, stored as no regions", regions)
	}

	// The second plan is priced region by region, written out of order and with a
	// region repeated: sorted by first region, de-duplicated.
	if len(machines.Plans[1].Prices) != 2 {
		t.Fatalf("EX-M has %d prices, want 2", len(machines.Plans[1].Prices))
	}

	if got := machines.Plans[1].Prices[0].Regions; !slices.Equal(got, []string{"xx-ams"}) {
		t.Errorf("EX-M's first price regions = %v, want [xx-ams]", got)
	}

	if got := machines.Plans[1].Prices[1].Regions; !slices.Equal(got, []string{"xx-par"}) {
		t.Errorf("EX-M's second price regions = %v, want [xx-par], the repeated region stored once", got)
	}

	var prices []catalog.Price

	for _, offering := range offerings {
		for _, plan := range offering.Plans {
			prices = append(prices, plan.Prices...)
		}

		prices = append(prices, offering.StoragePrices...)
	}

	if len(prices) == 0 {
		t.Fatal("the priced fixture holds no price at all")
	}

	for _, price := range prices {
		if price.Currency != catalog.EUR {
			t.Errorf("price of %s is in %q, want %q", price.Amount, price.Currency, catalog.EUR)
		}

		if price.Amount < 0 {
			t.Errorf("price of %s is negative, want at least 0", price.Amount)
		}
	}

	// The default region is one the catalog is available in, so an estimate in it
	// can find an offering.
	available := slices.ContainsFunc(offerings, func(o catalog.Offering) bool {
		return slices.Contains(o.Regions, testutil.PricedDefaultRegion)
	})
	if !available {
		t.Errorf("no offering is available in the default region %q", testutil.PricedDefaultRegion)
	}
}

// TestPricedOfferingsReturnsFreshSlices pins that the fixture builders return
// fresh slices, so one test changing a plan or a price cannot change what another
// test sees.
func TestPricedOfferingsReturnsFreshSlices(t *testing.T) {
	t.Parallel()

	first := testutil.PricedOfferings()
	first[0].Plans[0].Name = "Tampered"
	first[0].Plans[0].Prices[0].Amount = 1
	first[0].StoragePrices[0].Amount = 1

	second := testutil.PricedOfferings()
	if got := second[0].Plans[0].Name; got != "EX-L" {
		t.Errorf("plan name after changing an earlier result = %q, want %q", got, "EX-L")
	}

	if got := second[0].Plans[0].Prices[0].Amount; got != 520000 {
		t.Errorf("plan price after changing an earlier result = %s, want 5200.00", got)
	}

	if got := second[0].StoragePrices[0].Amount; got != 400 {
		t.Errorf("storage price after changing an earlier result = %s, want 0.04", got)
	}
}
