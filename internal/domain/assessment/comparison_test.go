package assessment_test

import (
	"slices"
	"strings"
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

// The keys a comparison names its targets by: the CLI's keys, not the catalogs'
// display names, so a test can tell a key from the target name in a reason.
const (
	ovhcloudKey = "ovhcloud"
	scalewayKey = "scaleway"
	upcloudKey  = "upcloud"
)

// comparisonInventory is the inventory the comparison tests assess: the one the
// assessment tests already pin every fit of.
func comparisonInventory(t *testing.T) inventory.Inventory {
	t.Helper()

	return newInventory(t, awsResources())
}

// comparisonCatalog returns the fixture catalog of one target key.
func comparisonCatalog(t *testing.T, key string) catalog.Catalog {
	t.Helper()

	switch key {
	case ovhcloudKey:
		return testutil.OVHcloudCatalog(t)
	case scalewayKey:
		return testutil.ScalewayCatalog(t)
	case upcloudKey:
		return testutil.UpCloudCatalog(t)
	default:
		t.Fatalf("no fixture catalog for target key %q", key)

		return catalog.Catalog{}
	}
}

// comparisonTargets assesses the comparison inventory against the given targets,
// in the order given: what a caller hands Compare before it sorts.
func comparisonTargets(t *testing.T, keys ...string) []assessment.TargetAssessment {
	t.Helper()

	inv := comparisonInventory(t)
	targets := make([]assessment.TargetAssessment, 0, len(keys))

	for _, key := range keys {
		targets = append(targets, assessment.TargetAssessment{
			Key:        key,
			Assessment: assessment.Assess(inv, comparisonCatalog(t, key)),
		})
	}

	return targets
}

// targetKeys returns the keys of targets or of a comparison's targets, in the
// order they hold.
func targetKeys(targets []assessment.TargetAssessment) []string {
	if len(targets) == 0 {
		return nil
	}

	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		keys = append(keys, target.Key)
	}

	return keys
}

// sortedTargets is the copy of targets Compare must produce: its own, in key
// order.
func sortedTargets(targets []assessment.TargetAssessment) []assessment.TargetAssessment {
	sorted := slices.Clone(targets)
	slices.SortFunc(sorted, func(a, b assessment.TargetAssessment) int {
		return strings.Compare(a.Key, b.Key)
	})

	return sorted
}

// blockerIDs returns the resource IDs of the shared blockers, in the order they
// hold.
func blockerIDs(blockers []assessment.SharedBlocker) []string {
	if len(blockers) == 0 {
		return nil
	}

	ids := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		ids = append(ids, blocker.ResourceID)
	}

	return ids
}

// sharedFromBlockers builds the shared blockers a single target's assessment
// implies: one per blocked resource, in the assessment's order, each carrying
// that target's reasons for it.
func sharedFromBlockers(key string, blockers []assessment.Blocker) []assessment.SharedBlocker {
	var shared []assessment.SharedBlocker

	for _, blocker := range blockers {
		if n := len(shared); n > 0 && shared[n-1].ResourceID == blocker.ResourceID {
			shared[n-1].Targets[0].Reasons = append(shared[n-1].Targets[0].Reasons, blocker.Reason)

			continue
		}

		shared = append(shared, assessment.SharedBlocker{
			ResourceID: blocker.ResourceID,
			Targets: []assessment.TargetReasons{
				{Key: key, Reasons: []mapping.Reason{blocker.Reason}},
			},
		})
	}

	return shared
}

// TestCompare pins what a comparison of several targets is: one assessment per
// target in key order, the caller's targets and assessments left as they were,
// and the blockers every target shares, with each target's own reasons.
func TestCompare(t *testing.T) {
	t.Parallel()

	// The comparison's target order, whatever order the caller used.
	sortedKeys := []string{ovhcloudKey, scalewayKey, upcloudKey}

	t.Run("sorts the targets by key", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			gave []string
		}{
			{name: "already in key order", gave: []string{ovhcloudKey, scalewayKey, upcloudKey}},
			{name: "reversed", gave: []string{upcloudKey, scalewayKey, ovhcloudKey}},
			{name: "in another order", gave: []string{upcloudKey, ovhcloudKey, scalewayKey}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				targets := comparisonTargets(t, tt.gave...)
				before := slices.Clone(targets)

				got := assessment.Compare(targets)

				if diff := cmp.Diff(sortedKeys, targetKeys(got.Targets)); diff != "" {
					t.Errorf("target keys mismatch (-want +got):\n%s", diff)
				}

				// The order the targets were given in changes nothing else either.
				want := assessment.Compare(comparisonTargets(t, sortedKeys...))
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("comparison mismatch (-want +got):\n%s", diff)
				}

				// Compare works on a copy: the caller's slice keeps its order.
				if diff := cmp.Diff(before, targets); diff != "" {
					t.Errorf("Compare() changed the caller's targets (-before +after):\n%s", diff)
				}
			})
		}
	})

	t.Run("leaves every assessment as it was", func(t *testing.T) {
		t.Parallel()

		targets := comparisonTargets(t, upcloudKey, ovhcloudKey, scalewayKey)

		got := assessment.Compare(targets)

		// The same values the caller handed in, in key order: Compare copies the
		// assessments and changes none of their fields, not even a verdict.
		want := sortedTargets(targets)
		if diff := cmp.Diff(want, got.Targets); diff != "" {
			t.Errorf("comparison targets mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("reports the blockers every target shares", func(t *testing.T) {
		t.Parallel()

		targets := comparisonTargets(t, upcloudKey, ovhcloudKey, scalewayKey)

		// A blocker on one target only is not shared: UpCloud serves no PostgreSQL
		// catalog offering, so db-example is blocked there and nowhere else.
		upcloud := assessment.Assess(comparisonInventory(t), testutil.UpCloudCatalog(t))
		if fit := verdictFor(t, upcloud.Verdicts, "db-example").Fit; fit != mapping.None {
			t.Fatalf("precondition failed: db-example fit on %s = %q, want %q", upcloudKey, fit, mapping.None)
		}

		got := assessment.Compare(targets)

		// One entry per resource blocked on every target, in resource ID order,
		// with each target's own reasons in target key order.
		want := []assessment.SharedBlocker{
			{
				ResourceID: "fn-example",
				Targets: []assessment.TargetReasons{
					{
						Key: ovhcloudKey,
						Reasons: []mapping.Reason{{
							Code:     mapping.NoOffering,
							Target:   "OVHcloud",
							Category: taxonomy.ServerlessFunction,
						}},
					},
					{
						Key: scalewayKey,
						Reasons: []mapping.Reason{{
							Code:     mapping.NoOffering,
							Target:   "Scaleway",
							Category: taxonomy.ServerlessFunction,
						}},
					},
					{
						Key: upcloudKey,
						Reasons: []mapping.Reason{{
							Code:     mapping.NoOffering,
							Target:   "UpCloud",
							Category: taxonomy.ServerlessFunction,
						}},
					},
				},
			},
			{
				ResourceID: "misc-example",
				Targets: []assessment.TargetReasons{
					{Key: ovhcloudKey, Reasons: []mapping.Reason{{Code: mapping.NotClassified}}},
					{Key: scalewayKey, Reasons: []mapping.Reason{{Code: mapping.NotClassified}}},
					{Key: upcloudKey, Reasons: []mapping.Reason{{Code: mapping.NotClassified}}},
				},
			},
		}

		if diff := cmp.Diff(want, got.SharedBlockers); diff != "" {
			t.Errorf("SharedBlockers mismatch (-want +got):\n%s", diff)
		}

		if diff := cmp.Diff([]string{"fn-example", "misc-example"}, blockerIDs(got.SharedBlockers)); diff != "" {
			t.Errorf("shared blocker IDs mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestComparisonValidate pins the preconditions a comparison of several targets
// must meet before a reporter lays it out: every target assesses the same
// resources in the same order (the one inventory's order), and every shared
// blocker names only compared targets. Both reporters refuse a comparison that
// breaks them, so the check lives here, in the domain, and not in one of them.
func TestComparisonValidate(t *testing.T) {
	t.Parallel()

	// The fixture comparison has shared blockers to break, and its targets
	// assess the same inventory in the same order.
	if len(assessment.Compare(comparisonTargets(t, ovhcloudKey, scalewayKey, upcloudKey)).SharedBlockers) == 0 {
		t.Fatal("precondition failed: the fixture comparison has no shared blocker")
	}

	tests := []struct {
		name   string
		change func(t *testing.T, c *assessment.Comparison)
		// wantErr is true when Validate must refuse the comparison.
		wantErr bool
	}{
		{
			name:   "every target assesses the same resources in the same order",
			change: func(*testing.T, *assessment.Comparison) {},
		},
		{
			name: "a later target holds more verdicts than the first",
			change: func(t *testing.T, c *assessment.Comparison) {
				t.Helper()

				c.Targets[1].Assessment.Verdicts = append(slices.Clone(c.Targets[1].Assessment.Verdicts),
					validationExtraVerdict())
			},
			wantErr: true,
		},
		{
			name: "a later target holds fewer verdicts than the first",
			change: func(_ *testing.T, c *assessment.Comparison) {
				c.Targets[1].Assessment.Verdicts = c.Targets[1].Assessment.Verdicts[1:]
			},
			wantErr: true,
		},
		{
			name: "the targets report on the same resources in another order",
			change: func(_ *testing.T, c *assessment.Comparison) {
				c.Targets[1].Assessment.Verdicts[0], c.Targets[1].Assessment.Verdicts[1] =
					c.Targets[1].Assessment.Verdicts[1], c.Targets[1].Assessment.Verdicts[0]
			},
			wantErr: true,
		},
		{
			name: "a shared blocker names a target the comparison does not hold",
			change: func(t *testing.T, c *assessment.Comparison) {
				t.Helper()

				c.SharedBlockers[0].Targets = append(c.SharedBlockers[0].Targets, assessment.TargetReasons{
					Key:     "missing",
					Reasons: []mapping.Reason{{Code: mapping.NotClassified}},
				})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := assessment.Compare(comparisonTargets(t, ovhcloudKey, scalewayKey, upcloudKey))
			tt.change(t, &c)

			err := c.Validate()

			switch {
			case tt.wantErr && err == nil:
				t.Fatalf("Validate() = nil, want an error for a comparison whose %s", tt.name)
			case !tt.wantErr && err != nil:
				t.Errorf("Validate() = %v, want nil for a comparison whose %s", err, tt.name)
			}
		})
	}
}

// validationExtraVerdict is a verdict on a resource the comparison inventory
// does not hold, for the target that must report on other resources.
func validationExtraVerdict() assessment.Verdict {
	return assessment.Verdict{
		Resource: inventory.Resource{
			ID:       "extra-example",
			RawType:  "AWS::SQS::Queue",
			Category: taxonomy.Unclassified,
		},
		Fit:     mapping.None,
		Reasons: []mapping.Reason{{Code: mapping.NotClassified}},
	}
}

// TestCompareNoTargets pins that comparing nothing is not an error and holds
// nothing: no assessments, and no shared blockers rather than an empty list.
func TestCompareNoTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		targets []assessment.TargetAssessment
	}{
		{name: "nil targets"},
		{name: "no targets", targets: []assessment.TargetAssessment{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := assessment.Compare(tt.targets)

			if got.Targets != nil {
				t.Errorf("Targets = %v, want nil for a comparison of no targets", got.Targets)
			}

			if got.SharedBlockers != nil {
				t.Errorf("SharedBlockers = %v, want nil for a comparison of no targets", got.SharedBlockers)
			}
		})
	}
}

// TestCompareSingleTarget pins that a comparison of one target is that target's
// own blocked resources: the same resources, in the same order, with the same
// reasons.
func TestCompareSingleTarget(t *testing.T) {
	t.Parallel()

	targets := comparisonTargets(t, scalewayKey)

	want := sharedFromBlockers(scalewayKey, targets[0].Assessment.Blockers)
	if len(want) == 0 {
		t.Fatal("precondition failed: the fixture assessment has no blockers")
	}

	got := assessment.Compare(targets)

	if diff := cmp.Diff(want, got.SharedBlockers); diff != "" {
		t.Errorf("SharedBlockers mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(targets, got.Targets); diff != "" {
		t.Errorf("Targets mismatch (-want +got):\n%s", diff)
	}
}

// TestCompareNoSharedBlockers pins the nil rule: when no resource is blocked on
// every target, the comparison holds nil shared blockers, not an empty slice.
func TestCompareNoSharedBlockers(t *testing.T) {
	t.Parallel()

	// One object bucket: direct on Scaleway, partial on UpCloud (its object
	// storage has no versioning), blocked on neither.
	inv := newInventory(t, []inventory.Resource{
		{
			ID:           "bucket-example",
			RawType:      "AWS::S3::Bucket",
			Category:     taxonomy.StorageObject,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.ObjectVersioning},
		},
	})

	targets := []assessment.TargetAssessment{
		{Key: scalewayKey, Assessment: assessment.Assess(inv, testutil.ScalewayCatalog(t))},
		{Key: upcloudKey, Assessment: assessment.Assess(inv, testutil.UpCloudCatalog(t))},
	}

	for _, target := range targets {
		if fit := verdictFor(t, target.Assessment.Verdicts, "bucket-example").Fit; fit == mapping.None {
			t.Fatalf("precondition failed: bucket-example fit on %s = %q, want better than %q",
				target.Key, fit, mapping.None)
		}
	}

	got := assessment.Compare(targets)

	if got.SharedBlockers != nil {
		t.Errorf("SharedBlockers = %v, want nil when no blocker is shared", got.SharedBlockers)
	}

	if diff := cmp.Diff([]string{scalewayKey, upcloudKey}, targetKeys(got.Targets)); diff != "" {
		t.Errorf("target keys mismatch (-want +got):\n%s", diff)
	}
}

// workloadComparisonInventory is one inventory with workloads, a relation onto a
// resource and the source's own monthly costs, assessed against two targets that
// block different resources: OVHcloud serves PostgreSQL, UpCloud sells no
// database at all (PRD #190, ADR 0028).
func workloadComparisonInventory(t *testing.T) inventory.Inventory {
	t.Helper()

	return newInventory(t, []inventory.Resource{
		{
			ID: "bucket-example", RawType: "AWS::S3::Bucket",
			Category: taxonomy.StorageObject, Region: "eu-west-1",
			Size:        &inventory.Size{StorageGiB: f(1000)},
			MonthlyCost: &inventory.Cost{Amount: f(1), Currency: "EUR"},
		},
		{
			ID: "db-example", RawType: "AWS::RDS::DBInstance",
			Category: taxonomy.DatabasePostgreSQL, Region: "eu-west-1",
			Workload:    "shop",
			MonthlyCost: &inventory.Cost{Amount: f(10), Currency: "EUR"},
		},
		{
			ID: "web-1", RawType: "AWS::EC2::Instance",
			Category: taxonomy.ComputeVM, Region: "eu-west-1",
			Size:        &inventory.Size{VCPUs: f(2), MemoryGiB: f(4)},
			Workload:    "shop",
			Relations:   []inventory.Relation{{Type: inventory.DependsOn, Target: "db-example"}},
			MonthlyCost: &inventory.Cost{Amount: f(20), Currency: "USD"},
		},
	})
}

// workloadShapes returns what every target of a comparison shares about its
// workload groups: the names, whether a group is the "No workload" one, and its
// resources. The counts, blockers and totals differ per target.
func workloadShapes(workloads []assessment.WorkloadSummary) []workloadShape {
	if len(workloads) == 0 {
		return nil
	}

	shapes := make([]workloadShape, 0, len(workloads))
	for _, w := range workloads {
		shapes = append(shapes, workloadShape{Name: w.Name, NoWorkload: w.NoWorkload, ResourceIDs: w.ResourceIDs})
	}

	return shapes
}

// workloadShape is the part of a workload group a comparison's targets must
// agree on.
type workloadShape struct {
	Name        string
	NoWorkload  bool
	ResourceIDs []string
}

// comparisonWorkloadTargets assesses workloadComparisonInventory against the
// OVHcloud and the UpCloud fixture, in key order.
func comparisonWorkloadTargets(t *testing.T) []assessment.TargetAssessment {
	t.Helper()

	inv := workloadComparisonInventory(t)

	return []assessment.TargetAssessment{
		{Key: ovhcloudKey, Assessment: assessment.Assess(inv, testutil.OVHcloudCatalog(t))},
		{Key: upcloudKey, Assessment: assessment.Assess(inv, testutil.UpCloudCatalog(t))},
	}
}

// TestComparisonWorkloadsNotesAndSourceCost pins what a comparison shares and what
// it does not: the workload groups' names and resources and the source's cost are
// the one inventory's, so every target holds the same, while the blockers, the
// counts and the notes are each target's own (ADR 0028).
func TestComparisonWorkloadsNotesAndSourceCost(t *testing.T) {
	t.Parallel()

	got := assessment.Compare(comparisonWorkloadTargets(t))

	if diff := cmp.Diff([]string{ovhcloudKey, upcloudKey}, targetKeys(got.Targets)); diff != "" {
		t.Fatalf("target keys mismatch (-want +got):\n%s", diff)
	}

	// Every target holds the one inventory's groups: one per workload, the "No
	// workload" group last.
	wantGroups := []workloadShape{
		{Name: "shop", ResourceIDs: []string{"db-example", "web-1"}},
		{NoWorkload: true, ResourceIDs: []string{"bucket-example"}},
	}

	// The source's own cost comes from the inventory, so no target changes it.
	wantCost := estimate.SourceCost{
		Currencies: []estimate.CurrencyCost{
			{Currency: "EUR", Monthly: estimate.Ratio(11, 1), Resources: 2},
			{Currency: "USD", Monthly: estimate.Ratio(20, 1), Resources: 1},
		},
		Covered:   3,
		Resources: 3,
	}

	for _, target := range got.Targets {
		if diff := cmp.Diff(wantGroups, workloadShapes(target.Assessment.Workloads)); diff != "" {
			t.Errorf("target %q does not hold the inventory's workload groups (-want +got):\n%s", target.Key, diff)
		}

		if diff := cmp.Diff(wantCost, target.Assessment.SourceCost); diff != "" {
			t.Errorf("target %q's SourceCost mismatch (-want +got):\n%s", target.Key, diff)
		}
	}

	// The blockers, the counts and the notes are each target's own: UpCloud
	// blocks the database, and the web server that depends on it is affected
	// there only.
	tests := []struct {
		key         string
		wantBlocked []string
		wantNotes   []string
	}{
		{key: ovhcloudKey},
		{key: upcloudKey, wantBlocked: []string{"db-example"}, wantNotes: []string{"db-example"}},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()

			var target assessment.TargetAssessment

			for _, have := range got.Targets {
				if have.Key == tc.key {
					target = have
				}
			}

			if target.Key == "" {
				t.Fatalf("the comparison holds no target %q", tc.key)
			}

			if diff := cmp.Diff(tc.wantBlocked, blockedIDs(target.Assessment.Blockers)); diff != "" {
				t.Errorf("target %q's blockers mismatch (-want +got):\n%s", tc.key, diff)
			}

			if diff := cmp.Diff(tc.wantNotes, verdictFor(t, target.Assessment.Verdicts, "web-1").AffectedBy); diff != "" {
				t.Errorf("target %q's notes on web-1 mismatch (-want +got):\n%s", tc.key, diff)
			}
		})
	}

	if err := got.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil for targets that hold the same workload groups", err)
	}
}

// TestComparisonValidateWorkloads pins the workload precondition of a comparison:
// every target holds the first target's groups in the same order, with the same
// Name, NoWorkload and ResourceIDs, and Validate names the offending target when
// one does not (ADR 0028).
func TestComparisonValidateWorkloads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(t *testing.T, c *assessment.Comparison)
		// wantErr is true when Validate must refuse the comparison.
		wantErr bool
		// wantErrNames are the values the error must name: the target key, and
		// the workload the targets disagree on where both call it the same.
		wantErrNames []string
	}{
		{
			name:   "every target holds the inventory's groups",
			change: func(*testing.T, *assessment.Comparison) {},
		},
		{
			name: "a later target's group lists another resource",
			change: func(_ *testing.T, c *assessment.Comparison) {
				c.Targets[1].Assessment.Workloads[0].ResourceIDs = []string{"db-example"}
			},
			wantErr:      true,
			wantErrNames: []string{upcloudKey, "shop"},
		},
		{
			name: "a later target's group holds one resource more",
			change: func(_ *testing.T, c *assessment.Comparison) {
				c.Targets[1].Assessment.Workloads[0].ResourceIDs = []string{
					"db-example", "web-1", "extra-example",
				}
			},
			wantErr:      true,
			wantErrNames: []string{upcloudKey, "shop"},
		},
		{
			name: "a later target's group lists its resources in another order",
			change: func(_ *testing.T, c *assessment.Comparison) {
				ids := c.Targets[1].Assessment.Workloads[0].ResourceIDs
				ids[0], ids[1] = ids[1], ids[0]
			},
			wantErr:      true,
			wantErrNames: []string{upcloudKey, "shop"},
		},
		{
			name: "a later target's group carries another name",
			change: func(_ *testing.T, c *assessment.Comparison) {
				c.Targets[1].Assessment.Workloads[0].Name = "shopping"
			},
			wantErr:      true,
			wantErrNames: []string{upcloudKey},
		},
		{
			name: "a later target's group is no longer the No workload group",
			change: func(_ *testing.T, c *assessment.Comparison) {
				c.Targets[1].Assessment.Workloads[1].NoWorkload = false
			},
			wantErr:      true,
			wantErrNames: []string{upcloudKey},
		},
		{
			name: "a later target's No workload group holds another resource",
			change: func(_ *testing.T, c *assessment.Comparison) {
				c.Targets[1].Assessment.Workloads[1].ResourceIDs = []string{"extra-example"}
			},
			wantErr:      true,
			wantErrNames: []string{upcloudKey},
		},
		{
			name: "a later target holds one workload group more",
			change: func(_ *testing.T, c *assessment.Comparison) {
				c.Targets[1].Assessment.Workloads = append(c.Targets[1].Assessment.Workloads,
					assessment.WorkloadSummary{Name: "blog", ResourceIDs: []string{"bucket-example"}})
			},
			wantErr:      true,
			wantErrNames: []string{upcloudKey},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := assessment.Compare(comparisonWorkloadTargets(t))
			tt.change(t, &c)

			err := c.Validate()

			switch {
			case tt.wantErr && err == nil:
				t.Fatalf("Validate() = nil, want an error for a comparison whose %s", tt.name)
			case !tt.wantErr && err != nil:
				t.Fatalf("Validate() = %v, want nil for a comparison whose %s", err, tt.name)
			}

			if err == nil {
				return
			}

			for _, want := range tt.wantErrNames {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Validate() error %q does not name %q", err.Error(), want)
				}
			}
		})
	}
}
