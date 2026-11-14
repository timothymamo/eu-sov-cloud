package assessment_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/internal/domain/assessment"
	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/estimate"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
	"github.com/timothymamo/shift2eu/internal/domain/mapping"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/testutil"
)

// f is a given float64, as an inventory size carries it.
func f(v float64) *float64 { return &v }

// sizedResources is an inventory whose two resources carry sizes, so an
// assessment with estimates on has something to price: a compute.vm the priced
// fixture sells in both of its regions, and a storage.object it sells in one.
func sizedResources() []inventory.Resource {
	return []inventory.Resource{
		{
			ID:       "bucket-example",
			RawType:  "AWS::S3::Bucket",
			Category: taxonomy.StorageObject,
			Region:   "eu-west-1",
			Size:     &inventory.Size{StorageGiB: f(1000)},
		},
		{
			ID:       "i-0123456789abcdef0",
			RawType:  "AWS::EC2::Instance",
			Category: taxonomy.ComputeVM,
			Region:   "eu-west-1",
			Size:     &inventory.Size{VCPUs: f(2), MemoryGiB: f(4)},
		},
	}
}

// sizelessResources is sizedResources without its sizes: the same resources, as
// an inventory written without the version 2 fields has them.
func sizelessResources() []inventory.Resource {
	out := sizedResources()
	for i := range out {
		out[i].Size = nil
		out[i].Usage = nil
	}

	return out
}

// pricedCatalog is the fixture with invented plans and prices, sold in xx-par
// (also its default region) and, for compute, in xx-ams too.
func pricedCatalog(t *testing.T) catalog.Catalog {
	t.Helper()

	return testutil.PricedCatalog(t)
}

// scalewayCatalog is the fixture without plans, which a direct fit still cannot
// price.
func scalewayCatalog(t *testing.T) catalog.Catalog {
	t.Helper()

	return testutil.ScalewayCatalog(t)
}

// estimateFor returns the estimate of that resource's verdict, failing the test
// when the resource was dropped or carries no estimate at all.
func estimateFor(t *testing.T, verdicts []assessment.Verdict, id string) estimate.Estimate {
	t.Helper()

	v := verdictFor(t, verdicts, id)
	if v.Estimate == nil {
		t.Fatalf("the verdict of %q carries no estimate, want one on every verdict when estimates are on", id)
	}

	return *v.Estimate
}

// TestAssessWithZeroOptionsEqualsAssess pins that the zero Options is today's
// assessment: every region, no estimates (ADR 0027).
func TestAssessWithZeroOptionsEqualsAssess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		resources []inventory.Resource
		catalog   func(t *testing.T) catalog.Catalog
	}{
		{
			name:      "the Scaleway fixture, which sells no plan",
			resources: awsResources(),
			catalog:   scalewayCatalog,
		},
		{
			name:      "the priced fixture with sized resources",
			resources: sizedResources(),
			catalog:   pricedCatalog,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inv := newInventory(t, tc.resources)
			cat := tc.catalog(t)

			want := assessment.Assess(inv, cat)
			got := assessment.AssessWith(inv, cat, assessment.Options{})

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("AssessWith() with the zero Options differs from Assess() (-want +got):\n%s", diff)
			}
		})
	}
}

// TestAssessWithEstimates pins what estimates on add to an assessment judged
// across every region: an estimate on every verdict in the snapshot's default
// region, that region on the assessment, and the exact total of the estimates.
func TestAssessWithEstimates(t *testing.T) {
	t.Parallel()

	inv := newInventory(t, sizedResources())
	cat := pricedCatalog(t)

	got := assessment.AssessWith(inv, cat, assessment.Options{Estimate: true})

	if got.Region != "" {
		t.Errorf("Region = %q, want \"\" for an assessment across every region", got.Region)
	}

	if want := cat.Snapshot().DefaultRegion; got.EstimateRegion != want {
		t.Errorf("EstimateRegion = %q, want the snapshot's default region %q", got.EstimateRegion, want)
	}

	// The fits and the summary are the mapping's, unchanged by the estimates.
	want := assessment.Assess(inv, cat)

	if diff := cmp.Diff(want.Summary, got.Summary); diff != "" {
		t.Errorf("Summary mismatch (-Assess +AssessWith):\n%s", diff)
	}

	for i, v := range got.Verdicts {
		if v.Fit != want.Verdicts[i].Fit {
			t.Errorf("verdict %q fit = %q, want the mapping's %q", v.Resource.ID, v.Fit, want.Verdicts[i].Fit)
		}

		if v.Estimate == nil {
			t.Fatalf("verdict %q carries no estimate, want one on every verdict", v.Resource.ID)
		}

		if want := cat.Snapshot().DefaultRegion; v.Estimate.Region != want {
			t.Errorf("verdict %q estimate region = %q, want the assessment's estimate region %q",
				v.Resource.ID, v.Estimate.Region, want)
		}
	}

	// The compute resource runs on the cheapest covering plan of the default
	// region; the bucket is priced per stored GiB.
	money := map[string]string{
		"bucket-example":      "15.00",
		"i-0123456789abcdef0": "25.00",
	}

	for id, wantMonthly := range money {
		e := estimateFor(t, got.Verdicts, id)

		if !e.Estimated {
			t.Errorf("the verdict of %q is not estimated (%s), want an estimate", id, e.Reason)
		}

		if e.Monthly.String() != wantMonthly {
			t.Errorf("the estimate of %q = %s EUR per month, want %s", id, e.Monthly.String(), wantMonthly)
		}
	}

	wantTotal := &estimate.Total{Monthly: estimate.FromAmount(400000), Estimated: 2, Resources: 2}
	if diff := cmp.Diff(wantTotal, got.Total); diff != "" {
		t.Errorf("Total mismatch (-want +got):\n%s", diff)
	}

	// The total is the estimates' sum, in verdict order.
	estimates := make([]estimate.Estimate, 0, len(got.Verdicts))
	for _, v := range got.Verdicts {
		estimates = append(estimates, *v.Estimate)
	}

	sum := estimate.Sum(estimates)
	if diff := cmp.Diff(sum, *got.Total); diff != "" {
		t.Errorf("Total is not the sum of the verdicts' estimates (-sum +Total):\n%s", diff)
	}
}

// TestAssessWithRegion pins that a chosen region scopes availability and is the
// estimate region: an offering outside it is no candidate, so its resource is a
// blocker with the reason NotOfferedInRegion, and the estimates are for the
// chosen region's prices.
func TestAssessWithRegion(t *testing.T) {
	t.Parallel()

	type want struct {
		fits    map[string]mapping.Fit
		reasons map[string][]mapping.Reason
		monthly map[string]string
		total   estimate.Total
		blocked []string
	}

	tests := []struct {
		name   string
		region string
		want   want
	}{
		{
			// Both offerings are available in the default region.
			name:   "a region every offering serves",
			region: testutil.PricedDefaultRegion,
			want: want{
				fits: map[string]mapping.Fit{
					"bucket-example":      mapping.Direct,
					"i-0123456789abcdef0": mapping.Direct,
				},
				// A direct fit carries the AllCapabilities reason, as Map gives it.
				reasons: map[string][]mapping.Reason{
					"bucket-example":      {{Code: mapping.AllCapabilities, Offering: "Example Object Storage"}},
					"i-0123456789abcdef0": {{Code: mapping.AllCapabilities, Offering: "Example Virtual Machines"}},
				},
				monthly: map[string]string{
					"bucket-example":      "15.00",
					"i-0123456789abcdef0": "25.00",
				},
				total: estimate.Total{Monthly: estimate.FromAmount(400000), Estimated: 2, Resources: 2},
			},
		},
		{
			// The object storage offering is sold in xx-par only, so the bucket
			// is not offered in xx-ams; the compute plan is dearer there.
			name:   "a region one offering is not available in",
			region: "xx-ams",
			want: want{
				fits: map[string]mapping.Fit{
					"bucket-example":      mapping.None,
					"i-0123456789abcdef0": mapping.Direct,
				},
				reasons: map[string][]mapping.Reason{
					"bucket-example": {{
						Code:     mapping.NotOfferedInRegion,
						Target:   "Example Cloud",
						Category: taxonomy.StorageObject,
						Region:   "xx-ams",
					}},
					"i-0123456789abcdef0": {{Code: mapping.AllCapabilities, Offering: "Example Virtual Machines"}},
				},
				monthly: map[string]string{
					"i-0123456789abcdef0": "26.00",
				},
				total:   estimate.Total{Monthly: estimate.FromAmount(260000), Estimated: 1, Resources: 2},
				blocked: []string{"bucket-example"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inv := newInventory(t, sizedResources())
			cat := pricedCatalog(t)

			got := assessment.AssessWith(inv, cat, assessment.Options{Region: tc.region, Estimate: true})

			if got.Region != tc.region {
				t.Errorf("Region = %q, want the chosen region %q", got.Region, tc.region)
			}

			if got.EstimateRegion != tc.region {
				t.Errorf("EstimateRegion = %q, want the chosen region %q", got.EstimateRegion, tc.region)
			}

			for id, wantFit := range tc.want.fits {
				v := verdictFor(t, got.Verdicts, id)

				if v.Fit != wantFit {
					t.Errorf("verdict %q fit = %q, want %q", id, v.Fit, wantFit)
				}

				if diff := cmp.Diff(tc.want.reasons[id], v.Reasons); diff != "" {
					t.Errorf("verdict %q reasons mismatch (-want +got):\n%s", id, diff)
				}

				if v.Estimate == nil {
					t.Fatalf("verdict %q carries no estimate, want one on every verdict", id)
				}

				if v.Estimate.Region != tc.region {
					t.Errorf("verdict %q estimate region = %q, want %q", id, v.Estimate.Region, tc.region)
				}
			}

			for id, wantMonthly := range tc.want.monthly {
				if got := estimateFor(t, got.Verdicts, id).Monthly.String(); got != wantMonthly {
					t.Errorf("the estimate of %q = %s EUR per month, want %s", id, got, wantMonthly)
				}
			}

			if diff := cmp.Diff(&tc.want.total, got.Total); diff != "" {
				t.Errorf("Total mismatch (-want +got):\n%s", diff)
			}

			var blocked []string

			for _, b := range got.Blockers {
				blocked = append(blocked, b.ResourceID)
			}

			if diff := cmp.Diff(tc.want.blocked, blocked); diff != "" {
				t.Errorf("blockers mismatch (-want +got):\n%s", diff)
			}

			if got.Summary.Total != len(got.Verdicts) || got.Summary.Total != tc.want.total.Resources {
				t.Errorf("Summary.Total = %d, want one verdict per resource and %d resources",
					got.Summary.Total, tc.want.total.Resources)
			}
		})
	}
}

// TestAssessWithEstimatesOff pins the switch: with estimates off there is no
// plan, no estimate and no total, and the fits are exactly those of the same
// inventory with its sizes removed (ADR 0027).
func TestAssessWithEstimatesOff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		region string
	}{
		{name: "across every region"},
		{name: "in one chosen region", region: "xx-ams"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cat := pricedCatalog(t)

			got := assessment.AssessWith(newInventory(t, sizedResources()), cat,
				assessment.Options{Region: tc.region, Estimate: false})
			want := assessment.AssessWith(newInventory(t, sizelessResources()), cat,
				assessment.Options{Region: tc.region, Estimate: false})

			// One verdict per resource, estimated or not.
			if n := len(got.Verdicts); n != 2 {
				t.Fatalf("AssessWith() returned %d verdicts, want one per resource (2)", n)
			}

			if got.EstimateRegion != "" {
				t.Errorf("EstimateRegion = %q, want \"\" with estimates off", got.EstimateRegion)
			}

			if got.Total != nil {
				t.Errorf("Total = %+v, want nil with estimates off", *got.Total)
			}

			for _, v := range got.Verdicts {
				if v.Estimate != nil {
					t.Errorf("verdict %q carries an estimate (%s), want none with estimates off",
						v.Resource.ID, v.Estimate.Monthly.Exact())
				}
			}

			// The sizes reach neither a fit nor a blocker: the same resources
			// without them assess identically, apart from the size the verdict
			// still carries.
			compared := got
			compared.Verdicts = slices.Clone(got.Verdicts)

			for i := range compared.Verdicts {
				compared.Verdicts[i].Resource.Size = nil
				compared.Verdicts[i].Resource.Usage = nil
			}

			if diff := cmp.Diff(want, compared); diff != "" {
				t.Errorf("a sized inventory does not assess as itself without sizes (-without +with):\n%s", diff)
			}
		})
	}
}
