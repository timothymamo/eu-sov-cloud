package testutil

import (
	"testing"
	"time"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
)

// The two catalogs below exist so a comparison test has several targets, not
// only the Scaleway fixture. Like it, they are named after real EU cloud
// providers but hold invented data: no account, project, region, price or
// product name from a real catalog belongs in a test.
//
// Together with ScalewayOfferings they cover what a comparison has to tell apart:
// a category every target serves (storage.object, compute.vm), a category only
// some serve (database.postgresql is missing from UpCloudOfferings, so a
// PostgreSQL resource is a blocker on UpCloud only), a category no target serves
// (serverless.function, like the Scaleway fixture), a capability only some
// offerings provide (ObjectVersioning is missing from UpCloud's object storage,
// so an object bucket is partial there and direct elsewhere), and a target that
// serves a category the others do not (KubernetesManaged on UpCloud).
const (
	ovhcloudTarget    = "OVHcloud"
	ovhcloudSourceURL = "https://www.ovhcloud.com/en/public-cloud/prices/"
	upcloudTarget     = "UpCloud"
	upcloudSourceURL  = "https://upcloud.com/pricing/"
)

// OVHcloudSnapshot is where and when the OVHcloud fixture catalog was taken. The
// date differs from ScalewaySnapshot so a comparison test can tell two targets'
// snapshots apart.
func OVHcloudSnapshot() catalog.Snapshot {
	return catalog.Snapshot{
		SourceURL:   ovhcloudSourceURL,
		RetrievedOn: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
	}
}

// OVHcloudOfferings is a small OVHcloud catalog: it serves object storage with
// versioning, virtual machines with snapshots, and PostgreSQL, and nothing else.
// It returns a fresh slice on every call, so a test may mutate its result.
func OVHcloudOfferings() []catalog.Offering {
	return []catalog.Offering{
		{
			Name:         "Object Storage",
			Categories:   []taxonomy.Category{taxonomy.StorageObject},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6, taxonomy.ObjectVersioning},
			Regions:      []string{"gra", "sbg"},
		},
		{
			Name:         "Public Cloud Instances",
			Categories:   []taxonomy.Category{taxonomy.ComputeVM},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6, taxonomy.Snapshots},
			Regions:      []string{"gra", "sbg"},
		},
		{
			Name:         "Managed PostgreSQL",
			Categories:   []taxonomy.Category{taxonomy.DatabasePostgreSQL},
			Capabilities: []taxonomy.Capability{taxonomy.PointInTimeRestore, taxonomy.ReadReplicas},
			Regions:      []string{"gra", "sbg"},
		},
	}
}

// UpCloudSnapshot is where and when the UpCloud fixture catalog was taken. The
// date differs from the other fixtures so a comparison test can tell two
// targets' snapshots apart.
func UpCloudSnapshot() catalog.Snapshot {
	return catalog.Snapshot{
		SourceURL:   upcloudSourceURL,
		RetrievedOn: time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC),
	}
}

// UpCloudOfferings is a small UpCloud catalog: it serves object storage without
// versioning, virtual machines with snapshots, and Kubernetes, and no database.
// It returns a fresh slice on every call, so a test may mutate its result.
func UpCloudOfferings() []catalog.Offering {
	return []catalog.Offering{
		{
			Name:         "Object Storage",
			Categories:   []taxonomy.Category{taxonomy.StorageObject},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
			Regions:      []string{"de-fra1"},
		},
		{
			Name:         "Cloud Server",
			Categories:   []taxonomy.Category{taxonomy.ComputeVM},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6, taxonomy.Snapshots},
			Regions:      []string{"de-fra1"},
		},
		{
			Name:         "Managed Kubernetes",
			Categories:   []taxonomy.Category{taxonomy.KubernetesManaged},
			Capabilities: []taxonomy.Capability{taxonomy.Autoscaling},
			Regions:      []string{"de-fra1"},
		},
	}
}

// OVHcloudCatalog is the OVHcloud fixture catalog, built through catalog.New so
// tests receive the same canonical form the rest of the program does.
func OVHcloudCatalog(tb testing.TB) catalog.Catalog {
	tb.Helper()

	cat, err := catalog.New(ovhcloudTarget, OVHcloudSnapshot(), OVHcloudOfferings())
	if err != nil {
		tb.Fatalf("catalog.New() returned unexpected error: %v", err)
	}

	return cat
}

// UpCloudCatalog is the UpCloud fixture catalog, built through catalog.New so
// tests receive the same canonical form the rest of the program does.
func UpCloudCatalog(tb testing.TB) catalog.Catalog {
	tb.Helper()

	cat, err := catalog.New(upcloudTarget, UpCloudSnapshot(), UpCloudOfferings())
	if err != nil {
		tb.Fatalf("catalog.New() returned unexpected error: %v", err)
	}

	return cat
}
