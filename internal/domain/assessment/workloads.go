package assessment

import (
	"slices"
	"strings"

	"github.com/timothymamo/shift2eu/internal/domain/estimate"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
	"github.com/timothymamo/shift2eu/internal/domain/mapping"
)

// WorkloadSummary is the assessment of one workload: the assessment's own rules
// applied to the verdicts of the resources that name it (ADR 0028).
type WorkloadSummary struct {
	// Name is the workload's name, its runs of white space collapsed to one
	// ASCII space, and "" for the "No workload" group.
	Name string
	// NoWorkload is true for the group of resources that name no workload. A
	// workload of only white space counts as none: inventory.New refuses one,
	// but a resource built without it lands here.
	NoWorkload bool
	// ResourceIDs are the group's resources, in verdict order (ID order).
	ResourceIDs []string
	// Summary counts the group's verdicts per fit; Summary.Total is
	// len(ResourceIDs).
	Summary Summary
	// Blockers are the entries of Assessment.Blockers whose resource is in the
	// group, in their order there, and nil when there is none.
	Blockers []Blocker
	// Total is estimate.Sum over the group's verdicts' estimates, in verdict
	// order, and nil when estimates are off.
	Total *estimate.Total
	// SourceCost is estimate.SumSourceCost over the group's resources.
	SourceCost estimate.SourceCost
}

// ShowWorkloads reports whether a report shows a per-workload section: true
// when at least one resource names a workload, that is, when Workloads holds a
// group with NoWorkload false.
func (a Assessment) ShowWorkloads() bool {
	return slices.ContainsFunc(a.Workloads, func(w WorkloadSummary) bool { return !w.NoWorkload })
}

// workloadName is the group key of a workload: every run of white space
// replaced by one ASCII space.
func workloadName(workload string) string {
	return strings.Join(strings.Fields(workload), " ")
}

// workloads groups a's verdicts by workload, by name in byte order, then the
// "No workload" group last. It is nil when a has no verdict.
func workloads(a Assessment, estimated bool) []WorkloadSummary {
	if len(a.Verdicts) == 0 {
		return nil
	}

	// members holds, per workload name, the indices of its verdicts in verdict
	// order. The names are sorted once, after grouping.
	var (
		members = make(map[string][]int)
		loose   []int
	)

	for i, v := range a.Verdicts {
		name := workloadName(v.Resource.Workload)
		if name == "" {
			loose = append(loose, i)

			continue
		}

		members[name] = append(members[name], i)
	}

	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}

	slices.Sort(names)

	groups := make([]WorkloadSummary, 0, len(names)+1)

	for _, name := range names {
		groups = append(groups, workload(a, WorkloadSummary{Name: name}, members[name], estimated))
	}

	if len(loose) > 0 {
		groups = append(groups, workload(a, WorkloadSummary{NoWorkload: true}, loose, estimated))
	}

	groupBlockers(a, groups)

	return groups
}

// groupBlockers gives each group the entries of a.Blockers whose resource is
// in it, in their order there, in one pass over a.Blockers.
func groupBlockers(a Assessment, groups []WorkloadSummary) {
	if len(a.Blockers) == 0 {
		return
	}

	// in maps a resource ID to the groups that hold it, each once, in group
	// order. A valid inventory holds an ID once, so it names one group.
	in := make(map[string][]int, len(a.Verdicts))

	for g := range groups {
		for _, id := range groups[g].ResourceIDs {
			if at := in[id]; len(at) == 0 || at[len(at)-1] != g {
				in[id] = append(at, g)
			}
		}
	}

	for _, b := range a.Blockers {
		for _, g := range in[b.ResourceID] {
			groups[g].Blockers = append(groups[g].Blockers, b)
		}
	}
}

// workload fills group with the verdicts of a at the indices in members, all
// but its blockers, which groupBlockers adds.
func workload(a Assessment, group WorkloadSummary, members []int, estimated bool) WorkloadSummary {
	group.ResourceIDs = make([]string, 0, len(members))
	group.Summary.Total = len(members)

	resources := make([]inventory.Resource, 0, len(members))

	var estimates []estimate.Estimate

	for _, i := range members {
		v := a.Verdicts[i]
		group.ResourceIDs = append(group.ResourceIDs, v.Resource.ID)
		resources = append(resources, v.Resource)

		switch v.Fit {
		case mapping.Direct:
			group.Summary.Direct++
		case mapping.Partial:
			group.Summary.Partial++
		case mapping.None:
			group.Summary.None++
		}

		if estimated && v.Estimate != nil {
			estimates = append(estimates, *v.Estimate)
		}
	}

	if estimated {
		total := estimate.Sum(estimates)
		group.Total = &total
	}

	group.SourceCost = estimate.SumSourceCost(resources)

	return group
}

// noteAffected sets each verdict's AffectedBy: the targets of its depends-on and
// replica-of relations whose verdict has fit mapping.None, each once, sorted.
// Only direct relations count, and a target that names no verdict gives no
// note.
func noteAffected(verdicts []Verdict) {
	blocked := make(map[string]bool)

	for _, v := range verdicts {
		if v.Fit == mapping.None {
			blocked[v.Resource.ID] = true
		}
	}

	if len(blocked) == 0 {
		return
	}

	for i := range verdicts {
		var notes []string

		for _, rel := range verdicts[i].Resource.Relations {
			if rel.Type != inventory.DependsOn && rel.Type != inventory.ReplicaOf {
				continue
			}

			if blocked[rel.Target] {
				notes = append(notes, rel.Target)
			}
		}

		if notes != nil {
			slices.Sort(notes)
			verdicts[i].AffectedBy = slices.Compact(notes)
		}
	}
}

// verdictResources returns the resources of verdicts, in verdict order.
func verdictResources(verdicts []Verdict) []inventory.Resource {
	resources := make([]inventory.Resource, 0, len(verdicts))
	for _, v := range verdicts {
		resources = append(resources, v.Resource)
	}

	return resources
}
