package assessment_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/internal/domain/assessment"
	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
	"github.com/timothymamo/shift2eu/internal/domain/mapping"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/testutil"
)

// relationlessWorkloadResources is workloadResources without its relations: the
// same inventory as one that relates nothing.
func relationlessWorkloadResources() []inventory.Resource {
	out := workloadResources()
	for i := range out {
		out[i].Relations = nil
	}

	return out
}

// blockedIDs returns the resource IDs of the blockers, in the order they hold.
func blockedIDs(blockers []assessment.Blocker) []string {
	if len(blockers) == 0 {
		return nil
	}

	ids := make([]string, 0, len(blockers))
	for _, b := range blockers {
		ids = append(ids, b.ResourceID)
	}

	return ids
}

// TestAffectedBy pins the note a resource gets when it relates to a resource that
// cannot move (PRD #190, ADR 0028): only a depends-on or replica-of relation
// carries one, only from the relating resource to its target, only one step, once
// per blocked resource, in blocked resource ID order, also on a blocked resource,
// and in every scope.
func TestAffectedBy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		resources []inventory.Resource
		catalog   func(tb testing.TB) catalog.Catalog
		opts      assessment.Options
		// wantBlocked are the IDs of the blocked resources, in verdict order.
		wantBlocked []string
		// wantCodes is the reason code of each named blocker, when given.
		wantCodes map[string]mapping.ReasonCode
		// want is AffectedBy per resource ID: every resource of the inventory,
		// nil when it gets no note.
		want map[string][]string
	}{
		{
			// Only from the relating resource to its target: what blocks the
			// dependant says nothing about its dependency.
			name: "a resource that depends on a blocked one",
			resources: []inventory.Resource{
				{
					ID: "fn-example", RawType: "AWS::Lambda::Function",
					Category: taxonomy.ServerlessFunction, Region: "eu-west-1",
				},
				{
					ID: "web-1", RawType: "AWS::EC2::Instance",
					Category: taxonomy.ComputeVM, Region: "eu-west-1",
					Relations: []inventory.Relation{{Type: inventory.DependsOn, Target: "fn-example"}},
				},
			},
			catalog:     testutil.ScalewayCatalog,
			wantBlocked: []string{"fn-example"},
			wantCodes:   map[string]mapping.ReasonCode{"fn-example": mapping.NoOffering},
			want: map[string][]string{
				"fn-example": nil,
				"web-1":      {"fn-example"},
			},
		},
		{
			// A replica of a blocked primary gets the note, the primary does not:
			// UpCloud serves no PostgreSQL, so neither database can move.
			name: "a replica of a blocked resource",
			resources: []inventory.Resource{
				{
					ID: "db-primary", RawType: "AWS::RDS::DBInstance",
					Category: taxonomy.DatabasePostgreSQL, Region: "eu-west-1",
				},
				{
					ID: "db-replica", RawType: "AWS::RDS::DBInstance",
					Category: taxonomy.DatabasePostgreSQL, Region: "eu-west-1",
					Relations: []inventory.Relation{{Type: inventory.ReplicaOf, Target: "db-primary"}},
				},
			},
			catalog:     testutil.UpCloudCatalog,
			wantBlocked: []string{"db-primary", "db-replica"},
			want: map[string][]string{
				"db-primary": nil,
				"db-replica": {"db-primary"},
			},
		},
		{
			// A volume follows its server and a dead-letter queue does not serve
			// its queue: neither relation gives a note.
			name: "attached-to and dead-letter-queue-of give no note",
			resources: []inventory.Resource{
				{
					ID: "db-primary", RawType: "AWS::RDS::DBInstance",
					Category: taxonomy.DatabasePostgreSQL, Region: "eu-west-1",
				},
				{
					ID: "vol-data", RawType: "AWS::EC2::Volume",
					Category: taxonomy.StorageBlock, Region: "eu-west-1",
					Relations: []inventory.Relation{{Type: inventory.AttachedTo, Target: "db-primary"}},
				},
				{
					ID: "queue-orders", RawType: "AWS::SQS::Queue",
					Category: taxonomy.MessagingQueue, Region: "eu-west-1",
				},
				{
					ID: "queue-dlq", RawType: "AWS::SQS::Queue",
					Category: taxonomy.MessagingQueue, Region: "eu-west-1",
					Relations: []inventory.Relation{{Type: inventory.DeadLetterQueueOf, Target: "queue-orders"}},
				},
			},
			catalog:     testutil.UpCloudCatalog,
			wantBlocked: []string{"db-primary", "queue-dlq", "queue-orders", "vol-data"},
			want: map[string][]string{
				"db-primary":   nil,
				"queue-dlq":    nil,
				"queue-orders": nil,
				"vol-data":     nil,
			},
		},
		{
			// Only direct relations: web-2 is not blocked by its note, so web-1
			// gets nothing about the resource web-2 depends on.
			name: "the note is not transitive",
			resources: []inventory.Resource{
				{
					ID: "misc-example", RawType: "AWS::CloudFormation::CustomResource",
					Category: taxonomy.Unclassified, Region: "eu-west-1",
				},
				{
					ID: "web-2", RawType: "AWS::EC2::Instance",
					Category: taxonomy.ComputeVM, Region: "eu-west-1",
					Relations: []inventory.Relation{{Type: inventory.DependsOn, Target: "misc-example"}},
				},
				{
					ID: "web-1", RawType: "AWS::EC2::Instance",
					Category: taxonomy.ComputeVM, Region: "eu-west-1",
					Relations: []inventory.Relation{{Type: inventory.DependsOn, Target: "web-2"}},
				},
			},
			catalog:     testutil.ScalewayCatalog,
			wantBlocked: []string{"misc-example"},
			want: map[string][]string{
				"misc-example": nil,
				"web-1":        nil,
				"web-2":        {"misc-example"},
			},
		},
		{
			// A resource that both depends on and is a replica of the same
			// blocked resource names it once.
			name: "a blocked resource is noted once however many relations point at it",
			resources: []inventory.Resource{
				{
					ID: "misc-example", RawType: "AWS::CloudFormation::CustomResource",
					Category: taxonomy.Unclassified, Region: "eu-west-1",
				},
				{
					ID: "web-1", RawType: "AWS::EC2::Instance",
					Category: taxonomy.ComputeVM, Region: "eu-west-1",
					Relations: []inventory.Relation{
						{Type: inventory.ReplicaOf, Target: "misc-example"},
						{Type: inventory.DependsOn, Target: "misc-example"},
					},
				},
			},
			catalog:     testutil.ScalewayCatalog,
			wantBlocked: []string{"misc-example"},
			want: map[string][]string{
				"misc-example": nil,
				"web-1":        {"misc-example"},
			},
		},
		{
			// The inventory sorts relations by type then target, the notes by
			// the blocked resource's ID.
			name: "several blocked resources come in ID order",
			resources: []inventory.Resource{
				{
					ID: "m-misc", RawType: "AWS::CloudFormation::CustomResource",
					Category: taxonomy.Unclassified, Region: "eu-west-1",
				},
				{
					ID: "z-misc", RawType: "AWS::CloudFormation::CustomResource",
					Category: taxonomy.Unclassified, Region: "eu-west-1",
				},
				{
					ID: "a-misc", RawType: "AWS::CloudFormation::CustomResource",
					Category: taxonomy.Unclassified, Region: "eu-west-1",
				},
				{
					ID: "web-1", RawType: "AWS::EC2::Instance",
					Category: taxonomy.ComputeVM, Region: "eu-west-1",
					Relations: []inventory.Relation{
						{Type: inventory.DependsOn, Target: "z-misc"},
						{Type: inventory.ReplicaOf, Target: "a-misc"},
						{Type: inventory.DependsOn, Target: "m-misc"},
					},
				},
			},
			catalog:     testutil.ScalewayCatalog,
			wantBlocked: []string{"a-misc", "m-misc", "z-misc"},
			want: map[string][]string{
				"a-misc": nil,
				"m-misc": nil,
				"web-1":  {"a-misc", "m-misc", "z-misc"},
				"z-misc": nil,
			},
		},
		{
			// A blocked resource keeps its own blocker and gets the note.
			name: "a blocked resource gets a note too",
			resources: []inventory.Resource{
				{
					ID: "misc-a", RawType: "AWS::CloudFormation::CustomResource",
					Category: taxonomy.Unclassified, Region: "eu-west-1",
					Relations: []inventory.Relation{{Type: inventory.DependsOn, Target: "misc-b"}},
				},
				{
					ID: "misc-b", RawType: "AWS::CloudFormation::CustomResource",
					Category: taxonomy.Unclassified, Region: "eu-west-1",
				},
			},
			catalog:     testutil.ScalewayCatalog,
			wantBlocked: []string{"misc-a", "misc-b"},
			want: map[string][]string{
				"misc-a": {"misc-b"},
				"misc-b": nil,
			},
		},
		{
			// Relations may form a cycle: each blocked resource notes the other,
			// and the assessment does not hang.
			name: "a cycle of blocked resources",
			resources: []inventory.Resource{
				{
					ID: "misc-a", RawType: "AWS::CloudFormation::CustomResource",
					Category: taxonomy.Unclassified, Region: "eu-west-1",
					Relations: []inventory.Relation{{Type: inventory.DependsOn, Target: "misc-b"}},
				},
				{
					ID: "misc-b", RawType: "AWS::CloudFormation::CustomResource",
					Category: taxonomy.Unclassified, Region: "eu-west-1",
					Relations: []inventory.Relation{{Type: inventory.DependsOn, Target: "misc-a"}},
				},
			},
			catalog:     testutil.ScalewayCatalog,
			wantBlocked: []string{"misc-a", "misc-b"},
			want: map[string][]string{
				"misc-a": {"misc-b"},
				"misc-b": {"misc-a"},
			},
		},
		{
			// The priced fixture sells object storage in xx-par only, so the
			// region blocks the bucket and the estimate switch changes nothing
			// about the note.
			name: "a region that blocks the target gives the note",
			resources: []inventory.Resource{
				{
					ID: "bucket-example", RawType: "AWS::S3::Bucket",
					Category: taxonomy.StorageObject, Region: "eu-west-1",
					Size: &inventory.Size{StorageGiB: f(1000)},
				},
				{
					ID: "web-1", RawType: "AWS::EC2::Instance",
					Category: taxonomy.ComputeVM, Region: "eu-west-1",
					Size:      &inventory.Size{VCPUs: f(2), MemoryGiB: f(4)},
					Relations: []inventory.Relation{{Type: inventory.DependsOn, Target: "bucket-example"}},
				},
			},
			catalog:     testutil.PricedCatalog,
			opts:        assessment.Options{Region: "xx-ams"},
			wantBlocked: []string{"bucket-example"},
			wantCodes:   map[string]mapping.ReasonCode{"bucket-example": mapping.NotOfferedInRegion},
			want: map[string][]string{
				"bucket-example": nil,
				"web-1":          {"bucket-example"},
			},
		},
		{
			name: "a region that blocks the target, with estimates on",
			resources: []inventory.Resource{
				{
					ID: "bucket-example", RawType: "AWS::S3::Bucket",
					Category: taxonomy.StorageObject, Region: "eu-west-1",
					Size: &inventory.Size{StorageGiB: f(1000)},
				},
				{
					ID: "web-1", RawType: "AWS::EC2::Instance",
					Category: taxonomy.ComputeVM, Region: "eu-west-1",
					Size:      &inventory.Size{VCPUs: f(2), MemoryGiB: f(4)},
					Relations: []inventory.Relation{{Type: inventory.DependsOn, Target: "bucket-example"}},
				},
			},
			catalog:     testutil.PricedCatalog,
			opts:        assessment.Options{Region: "xx-ams", Estimate: true},
			wantBlocked: []string{"bucket-example"},
			wantCodes:   map[string]mapping.ReasonCode{"bucket-example": mapping.NotOfferedInRegion},
			want: map[string][]string{
				"bucket-example": nil,
				"web-1":          {"bucket-example"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := assessment.AssessWith(newInventory(t, tc.resources), tc.catalog(t), tc.opts)

			if diff := cmp.Diff(tc.wantBlocked, blockedIDs(got.Blockers)); diff != "" {
				t.Fatalf("the blocked resources mismatch (-want +got):\n%s", diff)
			}

			for id, wantCode := range tc.wantCodes {
				var blocker assessment.Blocker

				found := false

				for _, b := range got.Blockers {
					if b.ResourceID == id {
						blocker, found = b, true

						break
					}
				}

				if !found {
					t.Fatalf("no blocker names %q, want one: the note must come from that blocker", id)
				}

				if blocker.Reason.Code != wantCode {
					t.Errorf("the blocker of %q has code %q, want %q: the note must come from that blocker",
						id, blocker.Reason.Code, wantCode)
				}
			}

			notes := make(map[string][]string, len(got.Verdicts))
			for _, v := range got.Verdicts {
				notes[v.Resource.ID] = v.AffectedBy
			}

			if diff := cmp.Diff(tc.want, notes); diff != "" {
				t.Errorf("AffectedBy mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestAffectedByChangesNothingElse pins that the notes add no fit, no blocker
// entry, no summary count and no estimate: the same inventory with and without
// its relations assesses alike (ADR 0028: no fit changes).
func TestAffectedByChangesNothingElse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts assessment.Options
	}{
		{name: "estimates off"},
		{name: "estimates on", opts: assessment.Options{Estimate: true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cat := pricedCatalog(t)
			got := assessment.AssessWith(newInventory(t, workloadResources()), cat, tc.opts)
			want := assessment.AssessWith(newInventory(t, relationlessWorkloadResources()), cat, tc.opts)

			// The note is there, so the comparison is not vacuous: web-1 depends
			// on the database the priced fixture blocks.
			if diff := cmp.Diff([]string{"db-shop"}, verdictFor(t, got.Verdicts, "web-1").AffectedBy); diff != "" {
				t.Fatalf("web-1's AffectedBy mismatch (-want +got):\n%s", diff)
			}

			if len(got.Verdicts) != len(want.Verdicts) {
				t.Fatalf("got %d verdicts, want the %d of the same inventory", len(got.Verdicts), len(want.Verdicts))
			}

			if diff := cmp.Diff(want.Summary, got.Summary); diff != "" {
				t.Errorf("Summary mismatch (-without relations +with):\n%s", diff)
			}

			if diff := cmp.Diff(want.Blockers, got.Blockers); diff != "" {
				t.Errorf("Blockers mismatch (-without relations +with):\n%s", diff)
			}

			if diff := cmp.Diff(want.Total, got.Total); diff != "" {
				t.Errorf("Total mismatch (-without relations +with):\n%s", diff)
			}

			for i, w := range want.Verdicts {
				g := got.Verdicts[i]

				for _, diff := range []string{
					cmp.Diff(w.Fit, g.Fit),
					cmp.Diff(w.Reasons, g.Reasons),
					cmp.Diff(w.Candidates, g.Candidates),
					cmp.Diff(w.RequiredChanges, g.RequiredChanges),
					cmp.Diff(w.Estimate, g.Estimate),
				} {
					if diff != "" {
						t.Errorf("the verdict of %q differs with the relations (-without +with):\n%s", g.Resource.ID, diff)
					}
				}
			}

			// The workload groups count the same, and hold no more blockers.
			if len(got.Workloads) != len(want.Workloads) {
				t.Fatalf("got %d workload groups, want the %d of the same inventory",
					len(got.Workloads), len(want.Workloads))
			}

			for i, w := range want.Workloads {
				g := got.Workloads[i]

				if diff := cmp.Diff(w.Summary, g.Summary); diff != "" {
					t.Errorf("the Summary of workload group %q mismatch (-without relations +with):\n%s",
						groupLabel(g), diff)
				}

				if diff := cmp.Diff(w.Blockers, g.Blockers); diff != "" {
					t.Errorf("the Blockers of workload group %q mismatch (-without relations +with):\n%s",
						groupLabel(g), diff)
				}
			}
		})
	}
}
