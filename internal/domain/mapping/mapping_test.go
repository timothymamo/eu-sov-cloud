package mapping_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
)

// newResource is the only way to build a Resource, so a test hands Map the same
// canonical form the inventory does.
func newResource(t *testing.T, r inventory.Resource) inventory.Resource {
	t.Helper()

	inv, err := inventory.New("aws", []inventory.Resource{r})
	if err != nil {
		t.Fatalf("inventory.New() returned unexpected error: %v", err)
	}

	return inv.Resources()[0]
}

// newCatalog is the only way to build a Catalog, so a test hands Map the same
// canonical form the catalog does.
func newCatalog(t *testing.T, offerings []catalog.Offering) catalog.Catalog {
	t.Helper()

	snapshot := catalog.Snapshot{
		SourceURL:   "https://www.scaleway.com/en/pricing/",
		RetrievedOn: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}

	cat, err := catalog.New("Scaleway", snapshot, offerings)
	if err != nil {
		t.Fatalf("catalog.New() returned unexpected error: %v", err)
	}

	return cat
}

// scalewayOfferings is the catalog most cases start from, named after real
// Scaleway products.
func scalewayOfferings() []catalog.Offering {
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
			// Elastic Metal is a dedicated server: compute.vm with IPv6, but no
			// autoscaling.
			Name:         "Elastic Metal",
			Categories:   []taxonomy.Category{taxonomy.ComputeVM},
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
			Capabilities: []taxonomy.Capability{taxonomy.Autoscaling, taxonomy.HighAvailability},
			Regions:      []string{"fr-par"},
		},
	}
}

// computeOffering is a compute.vm offering sold in fr-par only, which is also how
// the region test shows that a source region is not compared with a target one.
func computeOffering(name string, capabilities ...taxonomy.Capability) catalog.Offering {
	return catalog.Offering{
		Name:         name,
		Categories:   []taxonomy.Category{taxonomy.ComputeVM},
		Capabilities: capabilities,
		Regions:      []string{"fr-par"},
	}
}

// computeResource is a resource of category compute.vm.
func computeResource(id string, capabilities ...taxonomy.Capability) inventory.Resource {
	return inventory.Resource{
		ID:           id,
		RawType:      "AWS::EC2::Instance",
		Category:     taxonomy.ComputeVM,
		Region:       "eu-west-1",
		Capabilities: capabilities,
	}
}

func TestMap(t *testing.T) {
	t.Parallel()

	real := scalewayOfferings()

	instances := real[1]
	elasticMetal := real[2]
	postgresMySQL := real[4]

	// Direct before a partial whose name sorts first.
	zuluDirect := computeOffering("Zulu Compute", taxonomy.IPv6)
	alphaPartial := computeOffering("Alpha Compute")

	// Two direct candidates, given out of name order.
	alphaDirect := computeOffering("Alpha Compute", taxonomy.IPv6)
	zuluDirectAgain := computeOffering("Zulu Compute", taxonomy.IPv6)

	// Two partial candidates where name order and missing-capability count
	// disagree: Alpha lacks three capabilities, Zulu one.
	alphaNoCapabilities := computeOffering("Alpha Compute")
	zuluTwoCapabilities := computeOffering("Zulu Compute", taxonomy.Autoscaling, taxonomy.IPv6)

	tests := []struct {
		name      string
		resource  inventory.Resource
		offerings []catalog.Offering
		wantFit   mapping.Fit
		wantCands []mapping.Candidate
		wantReas  []mapping.Reason
	}{
		{
			// An unclassified resource is a fact about the resource, not a
			// fallback: nothing in the catalog can change it.
			name: "unclassified resource",
			resource: inventory.Resource{
				ID:           "misc-example",
				RawType:      "AWS::EC2::Instance",
				Category:     taxonomy.Unclassified,
				Region:       "eu-west-1",
				Capabilities: []taxonomy.Capability{taxonomy.IPv6},
			},
			offerings: real,
			wantFit:   mapping.None,
			wantCands: nil,
			wantReas:  []mapping.Reason{{Code: mapping.NotClassified}},
		},
		{
			name: "no offering serves the category",
			resource: inventory.Resource{
				ID:       "fn-example",
				RawType:  "AWS::Lambda::Function",
				Category: taxonomy.ServerlessFunction,
				Region:   "eu-west-1",
			},
			offerings: real,
			wantFit:   mapping.None,
			wantCands: nil,
			wantReas: []mapping.Reason{{
				Code:     mapping.NoOffering,
				Target:   "Scaleway",
				Category: taxonomy.ServerlessFunction,
			}},
		},
		{
			name:      "every capability offered is direct",
			resource:  computeResource("i-0123456789abcdef0", taxonomy.IPv6),
			offerings: real,
			wantFit:   mapping.Direct,
			wantCands: []mapping.Candidate{
				{Offering: elasticMetal, Fit: mapping.Direct},
				{Offering: instances, Fit: mapping.Direct},
			},
			wantReas: []mapping.Reason{{Code: mapping.AllCapabilities, Offering: "Elastic Metal"}},
		},
		{
			// A resource that uses no capabilities is direct with any offering
			// that serves its category.
			name:      "resource using no capabilities is direct",
			resource:  computeResource("i-0123456789abcdef0"),
			offerings: real,
			wantFit:   mapping.Direct,
			wantCands: []mapping.Candidate{
				{Offering: elasticMetal, Fit: mapping.Direct},
				{Offering: instances, Fit: mapping.Direct},
			},
			wantReas: []mapping.Reason{{Code: mapping.AllCapabilities, Offering: "Elastic Metal"}},
		},
		{
			// A capability no candidate offers makes the fit partial, and the
			// reasons are the best candidate's missing capabilities in
			// capability order.
			name:      "missing capabilities make it partial",
			resource:  computeResource("i-0123456789abcdef0", taxonomy.Autoscaling, taxonomy.IPv6, taxonomy.Snapshots),
			offerings: real,
			wantFit:   mapping.Partial,
			wantCands: []mapping.Candidate{
				{
					Offering: elasticMetal,
					Fit:      mapping.Partial,
					Missing:  []taxonomy.Capability{taxonomy.Autoscaling, taxonomy.Snapshots},
				},
				{
					Offering: instances,
					Fit:      mapping.Partial,
					Missing:  []taxonomy.Capability{taxonomy.Snapshots},
				},
			},
			wantReas: []mapping.Reason{
				{Code: mapping.MissingCapability, Offering: "Elastic Metal", Capability: taxonomy.Autoscaling},
				{Code: mapping.MissingCapability, Offering: "Elastic Metal", Capability: taxonomy.Snapshots},
			},
		},
		{
			name:      "direct comes before a partial that sorts first",
			resource:  computeResource("i-0123456789abcdef0", taxonomy.IPv6),
			offerings: []catalog.Offering{zuluDirect, alphaPartial},
			wantFit:   mapping.Direct,
			wantCands: []mapping.Candidate{
				{Offering: zuluDirect, Fit: mapping.Direct},
				{Offering: alphaPartial, Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.IPv6}},
			},
			wantReas: []mapping.Reason{{Code: mapping.AllCapabilities, Offering: "Zulu Compute"}},
		},
		{
			name:      "two direct candidates are ordered by name",
			resource:  computeResource("i-0123456789abcdef0", taxonomy.IPv6),
			offerings: []catalog.Offering{zuluDirectAgain, alphaDirect},
			wantFit:   mapping.Direct,
			wantCands: []mapping.Candidate{
				{Offering: alphaDirect, Fit: mapping.Direct},
				{Offering: zuluDirectAgain, Fit: mapping.Direct},
			},
			wantReas: []mapping.Reason{{Code: mapping.AllCapabilities, Offering: "Alpha Compute"}},
		},
		{
			// Name order, never the number of missing capabilities.
			name:      "two partial candidates are ordered by name",
			resource:  computeResource("i-0123456789abcdef0", taxonomy.Autoscaling, taxonomy.IPv6, taxonomy.Snapshots),
			offerings: []catalog.Offering{zuluTwoCapabilities, alphaNoCapabilities},
			wantFit:   mapping.Partial,
			wantCands: []mapping.Candidate{
				{
					Offering: alphaNoCapabilities,
					Fit:      mapping.Partial,
					Missing:  []taxonomy.Capability{taxonomy.Autoscaling, taxonomy.IPv6, taxonomy.Snapshots},
				},
				{
					Offering: zuluTwoCapabilities,
					Fit:      mapping.Partial,
					Missing:  []taxonomy.Capability{taxonomy.Snapshots},
				},
			},
			wantReas: []mapping.Reason{
				{Code: mapping.MissingCapability, Offering: "Alpha Compute", Capability: taxonomy.Autoscaling},
				{Code: mapping.MissingCapability, Offering: "Alpha Compute", Capability: taxonomy.IPv6},
				{Code: mapping.MissingCapability, Offering: "Alpha Compute", Capability: taxonomy.Snapshots},
			},
		},
		{
			// MySQL is served by the one offering whose category list has it.
			name: "mysql served by a multi-category offering",
			resource: inventory.Resource{
				ID:           "db-example",
				RawType:      "AWS::RDS::DBInstance",
				Category:     taxonomy.DatabaseMySQL,
				Region:       "eu-west-1",
				Capabilities: []taxonomy.Capability{taxonomy.ReadReplicas},
			},
			offerings: real,
			wantFit:   mapping.Direct,
			wantCands: []mapping.Candidate{{Offering: postgresMySQL, Fit: mapping.Direct}},
			wantReas: []mapping.Reason{
				{Code: mapping.AllCapabilities, Offering: "Managed Database for PostgreSQL and MySQL"},
			},
		},
		{
			// The source region is never compared with the target regions.
			name:      "region is ignored",
			resource:  computeResource("i-0123456789abcdef0", taxonomy.IPv6),
			offerings: []catalog.Offering{computeOffering("Instances", taxonomy.IPv6)},
			wantFit:   mapping.Direct,
			wantCands: []mapping.Candidate{{Offering: computeOffering("Instances", taxonomy.IPv6), Fit: mapping.Direct}},
			wantReas:  []mapping.Reason{{Code: mapping.AllCapabilities, Offering: "Instances"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resource := newResource(t, tc.resource)
			cat := newCatalog(t, tc.offerings)

			got := mapping.Map(resource, cat)

			if diff := cmp.Diff(tc.wantFit, got.Fit); diff != "" {
				t.Errorf("Fit mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tc.wantCands, got.Candidates); diff != "" {
				t.Errorf("Candidates mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tc.wantReas, got.Reasons); diff != "" {
				t.Errorf("Reasons mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(resource, got.Resource); diff != "" {
				t.Errorf("Result.Resource mismatch (-want +got):\n%s", diff)
			}

			best, ok := got.Best()
			if ok != (len(got.Candidates) > 0) {
				t.Fatalf("Best() ok = %t with %d candidates", ok, len(got.Candidates))
			}

			if ok && best.Fit != got.Fit {
				t.Errorf("Best().Fit = %q, want Result.Fit %q", best.Fit, got.Fit)
			}
		})
	}
}

// Map only reads: neither the resource nor the catalog may change, because the
// inventory and the catalog are shared with the report.
func TestMapDoesNotMutateInputs(t *testing.T) {
	t.Parallel()

	resource := newResource(t, computeResource("i-0123456789abcdef0", taxonomy.Autoscaling, taxonomy.IPv6))
	cat := newCatalog(t, scalewayOfferings())

	wantResource := resource
	wantResource.Capabilities = slices.Clone(resource.Capabilities)

	wantOfferings := cat.Offerings()

	_ = mapping.Map(resource, cat)

	if diff := cmp.Diff(wantResource, resource); diff != "" {
		t.Errorf("resource changed by Map (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(wantOfferings, cat.Offerings()); diff != "" {
		t.Errorf("catalog changed by Map (-want +got):\n%s", diff)
	}
}

// Best is the first candidate: Map has already ordered them best first, so Best
// must not search again.
func TestResultBest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   mapping.Result
		want mapping.Candidate
		ok   bool
	}{
		{name: "no candidates", in: mapping.Result{Fit: mapping.None}, want: mapping.Candidate{}, ok: false},
		{
			name: "first candidate",
			in: mapping.Result{Fit: mapping.Partial, Candidates: []mapping.Candidate{
				{Fit: mapping.Partial, Offering: catalog.Offering{Name: "Alpha Compute"}},
				{Fit: mapping.Direct, Offering: catalog.Offering{Name: "Zulu Compute"}},
			}},
			want: mapping.Candidate{Fit: mapping.Partial, Offering: catalog.Offering{Name: "Alpha Compute"}},
			ok:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := tc.in.Best()
			if ok != tc.ok {
				t.Fatalf("Best() ok = %t, want %t", ok, tc.ok)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Best() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// Every reason a report shows is one of these sentences, so they are pinned
// exactly, backticks included.
func TestReasonString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   mapping.Reason
		want string
	}{
		{
			name: "not classified",
			in:   mapping.Reason{Code: mapping.NotClassified},
			want: "not classified",
		},
		{
			name: "no offering",
			in: mapping.Reason{
				Code:     mapping.NoOffering,
				Target:   "Scaleway",
				Category: taxonomy.ServerlessFunction,
			},
			want: "no offering in the Scaleway catalog for `serverless.function`",
		},
		{
			name: "all capabilities",
			in:   mapping.Reason{Code: mapping.AllCapabilities, Offering: "Instances"},
			want: "Instances offers every capability the resource uses",
		},
		{
			name: "missing capability",
			in: mapping.Reason{
				Code:       mapping.MissingCapability,
				Offering:   "Elastic Metal",
				Capability: taxonomy.PointInTimeRestore,
			},
			want: "Elastic Metal does not offer `point-in-time-restore`",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.in.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}
