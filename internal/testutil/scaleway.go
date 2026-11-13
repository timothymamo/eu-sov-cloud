package testutil

import (
	"testing"
	"time"

	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
)

// The fixture below is named after real Scaleway products and regions, but it is
// invented data: no account, subscription or project identifier, no address and
// no price from a real catalog belongs in a test.
const (
	scalewayTarget    = "Scaleway"
	scalewaySourceURL = "https://www.scaleway.com/en/pricing/"
)

// ScalewaySnapshot is where and when the Scaleway fixture catalog was taken.
func ScalewaySnapshot() catalog.Snapshot {
	return catalog.Snapshot{
		SourceURL:   scalewaySourceURL,
		RetrievedOn: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}
}

// ScalewayOfferings is a small Scaleway catalog covering the shapes the mapping
// rules care about: several offerings for one category, one offering serving
// several categories, and capabilities that some offerings lack. It returns a
// fresh slice on every call, so a test may mutate its result.
func ScalewayOfferings() []catalog.Offering {
	return []catalog.Offering{
		{
			Name:         "Object Storage",
			Categories:   []taxonomy.Category{taxonomy.StorageObject},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6, taxonomy.ObjectVersioning},
			Regions:      []string{"fr-par", "nl-ams"},
		},
		{
			Name:         "Instances",
			Categories:   []taxonomy.Category{taxonomy.ComputeVM},
			Capabilities: []taxonomy.Capability{taxonomy.Autoscaling, taxonomy.IPv6},
			Regions:      []string{"fr-par", "nl-ams", "pl-waw"},
		},
		{
			// Elastic Metal is a dedicated physical server: compute.baremetal,
			// never compute.vm, exactly as the real snapshot classifies it
			// (issue #187, ADR 0023). Instances is therefore this fixture's only
			// compute.vm offering, and a VM is never mapped onto bare metal.
			Name:         "Elastic Metal",
			Categories:   []taxonomy.Category{taxonomy.ComputeBareMetal},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
			Regions:      []string{"fr-par", "nl-ams"},
		},
		{
			Name:         "Managed Database for PostgreSQL",
			Categories:   []taxonomy.Category{taxonomy.DatabasePostgreSQL},
			Capabilities: []taxonomy.Capability{taxonomy.PointInTimeRestore, taxonomy.ReadReplicas},
			Regions:      []string{"fr-par", "nl-ams", "pl-waw"},
		},
		{
			// One offering serving several categories.
			Name:         "Managed Database for PostgreSQL and MySQL",
			Categories:   []taxonomy.Category{taxonomy.DatabaseMySQL, taxonomy.DatabasePostgreSQL},
			Capabilities: []taxonomy.Capability{taxonomy.PointInTimeRestore, taxonomy.ReadReplicas},
			Regions:      []string{"fr-par", "nl-ams", "pl-waw"},
		},
		{
			Name:         "Kapsule",
			Categories:   []taxonomy.Category{taxonomy.KubernetesManaged},
			Capabilities: []taxonomy.Capability{taxonomy.Autoscaling, taxonomy.NodeHighAvailability},
			Regions:      []string{"fr-par"},
		},
	}
}

// ScalewayCatalog is the Scaleway fixture catalog, built through catalog.New so
// tests receive the same canonical form the rest of the program does.
func ScalewayCatalog(tb testing.TB) catalog.Catalog {
	tb.Helper()

	cat, err := catalog.New(scalewayTarget, ScalewaySnapshot(), ScalewayOfferings())
	if err != nil {
		tb.Fatalf("catalog.New() returned unexpected error: %v", err)
	}

	return cat
}
