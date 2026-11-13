package testutil

import (
	"testing"
	"time"

	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
)

// The fixture below is what a test needs once a catalog carries plans and
// prices: several plans on one offering, a price that applies to every region, a
// price that applies to one region only, and a per-GiB storage price. Every name,
// region and amount is invented, so a test can never pass by agreeing with a real
// provider's price list.
const (
	pricedTarget    = "Example Cloud"
	pricedSourceURL = "https://www.example.com/pricing/"

	// PricedDefaultRegion is the region PricedSnapshot names as its default: the
	// region estimates use when the user names none.
	PricedDefaultRegion = "xx-par"
)

// PricedSnapshot is where and when the priced fixture catalog was taken, and the
// region its estimates use by default.
func PricedSnapshot() catalog.Snapshot {
	return catalog.Snapshot{
		SourceURL:     pricedSourceURL,
		RetrievedOn:   time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC),
		DefaultRegion: PricedDefaultRegion,
	}
}

// PricedOfferings is a small catalog that sells plans and prices: a compute
// offering with three plans, one with a price for every region and one for a
// region only, one with a price per region, and a per-GiB storage price on both
// offerings. The plans are in no particular order, one price's regions are
// repeated and the prices of a plan are written out of order, so a test sees the
// canonical form catalog.New gives them. It returns a fresh slice on every call,
// so a test may mutate its result.
func PricedOfferings() []catalog.Offering {
	return []catalog.Offering{
		{
			Name:         "Example Virtual Machines",
			Categories:   []taxonomy.Category{taxonomy.ComputeVM},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
			Regions:      []string{"xx-par", "xx-ams"},
			Plans: []catalog.Plan{
				{
					Name:               "EX-L",
					VCPUs:              4,
					MemoryMilliGiB:     8000,
					IncludedStorageGiB: 80,
					Prices: []catalog.Price{
						{Amount: 520000, Currency: catalog.EUR, Regions: []string{"xx-ams"}},
						{Amount: 500000, Currency: catalog.EUR},
					},
				},
				{
					Name:           "EX-S",
					VCPUs:          1,
					MemoryMilliGiB: 2000,
					Prices:         []catalog.Price{{Amount: 120000, Currency: catalog.EUR}},
				},
				{
					// A plan publishes memory to three fractional digits, so the
					// fixture holds one that needs them, and prices it region by
					// region in the order catalog.New must sort, one region repeated.
					Name:               "EX-M",
					VCPUs:              2,
					MemoryMilliGiB:     4500,
					IncludedStorageGiB: 40,
					Prices: []catalog.Price{
						{Amount: 250000, Currency: catalog.EUR, Regions: []string{"xx-par", "xx-par"}},
						{Amount: 260000, Currency: catalog.EUR, Regions: []string{"xx-ams"}},
					},
				},
			},
			StoragePrices: []catalog.Price{
				{Amount: 400, Currency: catalog.EUR},
			},
		},
		{
			Name:          "Example Object Storage",
			Categories:    []taxonomy.Category{taxonomy.StorageObject},
			Capabilities:  []taxonomy.Capability{taxonomy.IPv6},
			Regions:       []string{"xx-par"},
			StoragePrices: []catalog.Price{{Amount: 150, Currency: catalog.EUR}},
		},
	}
}

// PricedCatalog is the priced fixture catalog, built through catalog.New so tests
// receive the same canonical form the rest of the program does.
func PricedCatalog(tb testing.TB) catalog.Catalog {
	tb.Helper()

	cat, err := catalog.New(pricedTarget, PricedSnapshot(), PricedOfferings())
	if err != nil {
		tb.Fatalf("catalog.New() returned unexpected error: %v", err)
	}

	return cat
}
