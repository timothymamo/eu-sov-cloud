package assessment_test

import (
	"fmt"
	"testing"

	"github.com/timothymamo/shift2eu/internal/domain/assessment"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
	"github.com/timothymamo/shift2eu/internal/domain/mapping"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/testutil"
)

// maxResources is inventoryfile.MaxResources, the most resources an inventory
// file may hold, so the worst case an assessment can be handed. The domain layer
// may not import the adapter that declares it, so the number is written out
// here.
const maxResources = 50000

// manyWorkloadResources returns n unclassified resources, each in its own
// workload: IDs r-00001.. in ascending order, workload names w<0n>..w000001 in
// descending order, so the grouping meets a name it has not seen before at the
// front of its sorted names on every resource. An unclassified resource is
// blocking with mapping.NotClassified whatever the catalog offers, so each group
// holds exactly one blocker.
func manyWorkloadResources(n int) []inventory.Resource {
	resources := make([]inventory.Resource, 0, n)

	for i := range n {
		resources = append(resources, inventory.Resource{
			ID:       fmt.Sprintf("r-%05d", i+1),
			RawType:  "AWS::CloudFormation::CustomResource",
			Category: taxonomy.Unclassified,
			Region:   "eu-west-1",
			Workload: fmt.Sprintf("w%06d", n-i),
		})
	}

	return resources
}

// manyWorkloadInventory is manyWorkloadResources as the only constructor builds
// an inventory.
func manyWorkloadInventory(tb testing.TB, n int) inventory.Inventory {
	tb.Helper()

	return workloadInventoryOf(tb, manyWorkloadResources(n))
}

// workloadInventoryOf builds the inventory of resources, failing the test or
// benchmark when the fixture is invalid.
func workloadInventoryOf(tb testing.TB, resources []inventory.Resource) inventory.Inventory {
	tb.Helper()

	inv, err := inventory.New(sourceName, resources)
	if err != nil {
		tb.Fatalf("inventory.New() for %d resources returned unexpected error: %v", len(resources), err)
	}

	return inv
}

// TestAssessWithManyWorkloads pins the grouping in the worst case an inventory
// can hold: maxResources resources, each its own workload, every one of them
// unclassified and so a blocker, and the workload names in descending order.
// Every group must hold exactly its one resource, count it as none and carry its
// one blocker, the groups by name in byte order, the blockers in blocker order
// (ADR 0028). It checks correctness only; BenchmarkAssessWithManyWorkloads
// measures the cost.
func TestAssessWithManyWorkloads(t *testing.T) {
	t.Parallel()

	const n = maxResources

	got := assessment.Assess(manyWorkloadInventory(t, n), testutil.OVHcloudCatalog(t))

	if len(got.Workloads) != n {
		t.Fatalf("Assess() returned %d workload groups, want one per resource (%d)", len(got.Workloads), n)
	}

	if len(got.Blockers) != n {
		t.Fatalf("Assess() returned %d blockers, want one per resource (%d)", len(got.Blockers), n)
	}

	if got.ShowWorkloads() != true {
		t.Fatal("ShowWorkloads() = false, want true for an assessment in which every resource names a workload")
	}

	for j, group := range got.Workloads {
		// The groups come by name in byte order and the names run against the
		// resource IDs: group j is the workload w<j+1>, which resource
		// r<n-j> names, and that resource's blocker is the one at position
		// n-j-1 of the assessment's blockers.
		at := n - j
		wantName := fmt.Sprintf("w%06d", j+1)
		wantID := fmt.Sprintf("r-%05d", at)
		wantBlocker := assessment.Blocker{ResourceID: wantID, Reason: mapping.Reason{Code: mapping.NotClassified}}

		if group.NoWorkload || group.Name != wantName {
			t.Fatalf("Workloads[%d] is the group %q (NoWorkload %t), want %q",
				j, group.Name, group.NoWorkload, wantName)
		}

		if len(group.ResourceIDs) != 1 || group.ResourceIDs[0] != wantID {
			t.Fatalf("the group %q holds %v, want only %q", wantName, group.ResourceIDs, wantID)
		}

		if want := (assessment.Summary{Total: 1, None: 1}); group.Summary != want {
			t.Fatalf("the group %q counts %+v, want %+v", wantName, group.Summary, want)
		}

		if len(group.Blockers) != 1 || group.Blockers[0] != wantBlocker {
			t.Fatalf("the group %q holds the blockers %+v, want only %+v", wantName, group.Blockers, wantBlocker)
		}

		if have := got.Blockers[at-1]; have != wantBlocker {
			t.Fatalf("the assessment's blocker at position %d is %+v, want %+v", at-1, have, wantBlocker)
		}
	}
}
