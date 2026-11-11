package assessment

import (
	"slices"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
)

// Blocker is one reason a resource cannot move to the target at all.
type Blocker struct {
	// ResourceID is the ID of the resource that cannot move.
	ResourceID string
	// Reason is why it cannot move.
	Reason mapping.Reason
}

// Verdict is the assessment of one resource: its mapping, plus the best
// offering and the changes moving to it requires.
type Verdict struct {
	// Resource is the resource assessed.
	Resource inventory.Resource
	// Fit is the mapping's fit.
	Fit mapping.Fit
	// BestOffering is the name of the best candidate offering, or empty when
	// there is no candidate.
	BestOffering string
	// Candidates are the mapping's candidates, best first.
	Candidates []mapping.Candidate
	// Reasons are the mapping's reasons for Fit.
	Reasons []mapping.Reason
	// RequiredChanges lists the capabilities the resource uses that the best
	// offering lacks, sorted, and nil when there are none or there is no
	// candidate.
	RequiredChanges []taxonomy.Capability
}

// Ties returns the names of the other candidates a reader could take for the
// best one: those with the best candidate's fit and as many missing
// capabilities, in candidate order. The best candidate is the first one. Ties
// is nil when there is no such candidate. Every report format shows the same
// ties.
func (v Verdict) Ties() []string {
	if len(v.Candidates) == 0 {
		return nil
	}

	first := v.Candidates[0]

	var ties []string

	for _, c := range v.Candidates[1:] {
		if c.Fit == first.Fit && len(c.Missing) == len(first.Missing) {
			ties = append(ties, c.Offering.Name)
		}
	}

	return ties
}

// ChangesToList returns the required changes a report lists for the verdict:
// RequiredChanges for a partial fit, and nil for any other fit. Every report
// format lists the same changes.
func (v Verdict) ChangesToList() []taxonomy.Capability {
	if v.Fit != mapping.Partial {
		return nil
	}

	return v.RequiredChanges
}

// Summary counts the verdicts per fit.
type Summary struct {
	// Total is the number of resources assessed.
	Total int
	// Direct is the number of resources with fit mapping.Direct.
	Direct int
	// Partial is the number of resources with fit mapping.Partial.
	Partial int
	// None is the number of resources with fit mapping.None.
	None int
}

// Assessment is the result of assessing one inventory against one target's
// catalog.
type Assessment struct {
	// Target is the catalog's target display name.
	Target string
	// Snapshot is where and when the catalog's data was taken.
	Snapshot catalog.Snapshot
	// Source names the source the inventory was taken from.
	Source string
	// Verdicts holds one verdict per resource, in resource ID order.
	Verdicts []Verdict
	// Blockers holds one blocker per reason of every verdict with fit
	// mapping.None, in resource ID order, and is nil when there is none.
	Blockers []Blocker
	// Summary counts Verdicts per fit.
	Summary Summary
}

// Assess maps every resource of inv onto cat and gathers the verdicts, blockers
// and summary counts. Fits and reasons come from mapping.Map unchanged. Resources
// come from the inventory in ID order, so the same inventory and catalog always
// give the same assessment.
func Assess(inv inventory.Inventory, cat catalog.Catalog) Assessment {
	resources := inv.Resources()

	a := Assessment{
		Target:   cat.Target(),
		Snapshot: cat.Snapshot(),
		Source:   inv.Source(),
		Verdicts: make([]Verdict, 0, len(resources)),
		Summary:  Summary{Total: len(resources)},
	}

	for _, r := range resources {
		v := verdict(mapping.Map(r, cat))
		a.Verdicts = append(a.Verdicts, v)

		switch v.Fit {
		case mapping.Direct:
			a.Summary.Direct++
		case mapping.Partial:
			a.Summary.Partial++
		case mapping.None:
			a.Summary.None++

			for _, reason := range v.Reasons {
				a.Blockers = append(a.Blockers, Blocker{ResourceID: v.Resource.ID, Reason: reason})
			}
		}
	}

	return a
}

// verdict turns a mapping result into a verdict.
func verdict(result mapping.Result) Verdict {
	v := Verdict{
		Resource:   result.Resource,
		Fit:        result.Fit,
		Candidates: result.Candidates,
		Reasons:    result.Reasons,
	}

	if best, ok := result.Best(); ok {
		v.BestOffering = best.Offering.Name
		v.RequiredChanges = slices.Clone(best.Missing)
	}

	return v
}
