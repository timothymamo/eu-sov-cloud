package assessment_test

import (
	"testing"

	"github.com/timothymamo/shift2eu/internal/domain/assessment"
	"github.com/timothymamo/shift2eu/internal/testutil"
)

// BenchmarkAssessWithManyWorkloads measures the worst case an inventory can hand
// an assessment: maxResources unclassified resources, each its own workload, so
// every resource is a blocker and the grouping meets its names in descending
// order (PRD #190 M7, security audit). Grouping cost O(groups x blockers) here,
// tens of seconds for one target, so run it with -bench ManyWorkloads
// -benchtime 1x rather than the default -benchtime.
func BenchmarkAssessWithManyWorkloads(b *testing.B) {
	inv := manyWorkloadInventory(b, maxResources)
	cat := testutil.OVHcloudCatalog(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		assessment.Assess(inv, cat)
	}
}

// BenchmarkAssessWithManyWorkloadsOneGroup is the same inventory with every
// resource in one workload: one group over maxResources resources, so the
// grouping and the per-group blocker walk cost one pass each. The difference to
// BenchmarkAssessWithManyWorkloads is what a per-group pass over every blocker
// adds.
func BenchmarkAssessWithManyWorkloadsOneGroup(b *testing.B) {
	resources := manyWorkloadResources(maxResources)
	for i := range resources {
		resources[i].Workload = "shop"
	}

	inv := workloadInventoryOf(b, resources)
	cat := testutil.OVHcloudCatalog(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		assessment.Assess(inv, cat)
	}
}
