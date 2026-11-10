package mapping

import (
	"fmt"
	"slices"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
)

// Fit is how well a target can take over a resource.
type Fit string

const (
	// Direct means an offering serves the resource's category and offers every
	// capability the resource uses.
	Direct Fit = "direct"
	// Partial means an offering serves the resource's category but lacks at least
	// one capability the resource uses.
	Partial Fit = "partial"
	// None means no offering can take over the resource: it is unclassified, or no
	// offering serves its category.
	None Fit = "none"
)

// rank orders fits best first: Direct, then Partial, then None.
func (f Fit) rank() int {
	switch f {
	case Direct:
		return 0
	case Partial:
		return 1
	case None:
		return 2
	default:
		return 3
	}
}

// ReasonCode says which rule decided a fit.
type ReasonCode string

const (
	// NotClassified is the reason for a resource of category
	// taxonomy.Unclassified.
	NotClassified ReasonCode = "not-classified"
	// NoOffering is the reason for a resource whose category no offering in the
	// catalog serves.
	NoOffering ReasonCode = "no-offering"
	// AllCapabilities is the reason for a direct fit: the offering offers every
	// capability the resource uses.
	AllCapabilities ReasonCode = "all-capabilities"
	// MissingCapability is a reason for a partial fit: the offering does not offer
	// one capability the resource uses.
	MissingCapability ReasonCode = "missing-capability"
)

// Reason explains one part of a fit. Code says which rule applied; only the
// fields that rule needs are set.
type Reason struct {
	// Code is the rule that applied.
	Code ReasonCode
	// Target is the catalog's target display name, set for NoOffering.
	Target string
	// Category is the resource's category, set for NoOffering.
	Category taxonomy.Category
	// Offering is the offering's name, set for AllCapabilities and
	// MissingCapability.
	Offering string
	// Capability is the capability the offering lacks, set for MissingCapability.
	Capability taxonomy.Capability
}

// String returns the reason as the sentence a report shows. Category and
// capability names are wrapped in backticks; Target and Offering are inserted as
// they are, so escaping them for an output format is the reporter's job.
func (r Reason) String() string {
	switch r.Code {
	case NotClassified:
		return "not classified"
	case NoOffering:
		return fmt.Sprintf("no offering in the %s catalog for `%s`", r.Target, r.Category)
	case AllCapabilities:
		return r.Offering + " offers every capability the resource uses"
	case MissingCapability:
		return fmt.Sprintf("%s does not offer `%s`", r.Offering, r.Capability)
	default:
		return string(r.Code)
	}
}

// Candidate is one offering that serves a resource's category, with how well it
// fits the resource.
type Candidate struct {
	// Offering is the catalog offering.
	Offering catalog.Offering
	// Fit is Direct or Partial.
	Fit Fit
	// Missing lists the capabilities the resource uses that the offering does
	// not offer, sorted, and nil when there are none.
	Missing []taxonomy.Capability
}

// Result is the mapping of one resource onto one catalog.
type Result struct {
	// Resource is the resource that was mapped.
	Resource inventory.Resource
	// Fit is the fit of the best candidate, or None when there is no candidate.
	Fit Fit
	// Candidates lists every offering that serves the resource's category, best
	// first: Direct before Partial, then by offering name. It is nil when there
	// is none.
	Candidates []Candidate
	// Reasons explains Fit. For a direct or partial fit it is about the best
	// candidate only.
	Reasons []Reason
}

// Best returns the best candidate, which is the first one, and whether there is
// one.
func (r Result) Best() (Candidate, bool) {
	if len(r.Candidates) == 0 {
		return Candidate{}, false
	}

	return r.Candidates[0], true
}

// Map matches resource r against catalog c.
//
// An unclassified resource has fit None with the reason NotClassified, whatever
// the catalog holds. A resource whose category no offering serves has fit None
// with the reason NoOffering. Otherwise every offering serving the category is a
// candidate: Direct when it offers every capability the resource uses, Partial
// with the capabilities it lacks otherwise. Candidates are ordered Direct before
// Partial, then by offering name in byte order, never by how many capabilities
// they lack. The result's fit and reasons are those of the best candidate: one
// AllCapabilities reason for Direct, one MissingCapability reason per lacking
// capability for Partial.
//
// Regions are not compared: where a resource runs today says nothing about which
// target region it must move to. Map only reads r and c.
func Map(r inventory.Resource, c catalog.Catalog) Result {
	r.Capabilities = slices.Clone(r.Capabilities)
	result := Result{Resource: r, Fit: None}

	if r.Category == taxonomy.Unclassified {
		result.Reasons = []Reason{{Code: NotClassified}}

		return result
	}

	offerings := c.OfferingsFor(r.Category)
	if len(offerings) == 0 {
		result.Reasons = []Reason{{Code: NoOffering, Target: c.Target(), Category: r.Category}}

		return result
	}

	candidates := make([]Candidate, 0, len(offerings))
	for _, o := range offerings {
		candidates = append(candidates, candidate(r, o))
	}

	slices.SortStableFunc(candidates, func(a, b Candidate) int {
		if d := a.Fit.rank() - b.Fit.rank(); d != 0 {
			return d
		}

		return strings.Compare(a.Offering.Name, b.Offering.Name)
	})

	best := candidates[0]
	result.Fit = best.Fit
	result.Candidates = candidates
	result.Reasons = reasons(best)

	return result
}

// candidate returns how offering o fits resource r.
func candidate(r inventory.Resource, o catalog.Offering) Candidate {
	var missing []taxonomy.Capability

	for _, capability := range r.Capabilities {
		if !o.Has(capability) {
			missing = append(missing, capability)
		}
	}

	if len(missing) == 0 {
		return Candidate{Offering: o, Fit: Direct}
	}

	slices.Sort(missing)

	return Candidate{Offering: o, Fit: Partial, Missing: slices.Compact(missing)}
}

// reasons explains the fit of the best candidate.
func reasons(best Candidate) []Reason {
	if best.Fit == Direct {
		return []Reason{{Code: AllCapabilities, Offering: best.Offering.Name}}
	}

	out := make([]Reason, 0, len(best.Missing))
	for _, capability := range best.Missing {
		out = append(out, Reason{Code: MissingCapability, Offering: best.Offering.Name, Capability: capability})
	}

	return out
}
