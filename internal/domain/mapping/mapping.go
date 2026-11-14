package mapping

import (
	"slices"
	"strings"

	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
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
	// NotOfferedInRegion is the reason for a resource whose category some
	// offering serves, but none in the region the assessment is scoped to
	// (MapIn). Its sentence is "not offered in <region>" (PRD #190, ADR 0027).
	NotOfferedInRegion ReasonCode = "not-offered-in-region"
)

// Reason explains one part of a fit. Code says which rule applied; only the
// fields that rule needs are set.
type Reason struct {
	// Code is the rule that applied.
	Code ReasonCode
	// Target is the catalog's target display name, set for NoOffering and
	// NotOfferedInRegion.
	Target string
	// Category is the resource's category, set for NoOffering and
	// NotOfferedInRegion.
	Category taxonomy.Category
	// Offering is the offering's name, set for AllCapabilities and
	// MissingCapability.
	Offering string
	// Capability is the capability the offering lacks, set for MissingCapability.
	Capability taxonomy.Capability
	// Region is the region the assessment is scoped to, set for
	// NotOfferedInRegion.
	Region string
}

// PartKind says what a Part of a reason is: sentence text, or one of the names
// the reason mentions.
type PartKind string

const (
	// PartLiteral is sentence text, or an unknown reason code verbatim; renderers
	// escape it as text; every other part is a value.
	PartLiteral PartKind = "literal"
	// PartTarget is a catalog's target display name.
	PartTarget PartKind = "target"
	// PartOffering is an offering's name.
	PartOffering PartKind = "offering"
	// PartCategory is a taxonomy category.
	PartCategory PartKind = "category"
	// PartCapability is a taxonomy capability.
	PartCapability PartKind = "capability"
	// PartRegion is a target region's name, as the catalog spells it.
	PartRegion PartKind = "region"
)

// Part is one piece of a reason's sentence. Text is exactly as the domain holds
// it, never escaped for any output format.
type Part struct {
	// Kind says what Text is.
	Kind PartKind
	// Text is the piece of the sentence.
	Text string
}

// Parts returns the reason's sentence split at the names it mentions, so a
// renderer can escape or style every name for its own format without parsing the
// sentence. A reason with an unknown Code is one literal part holding the code.
func (r Reason) Parts() []Part {
	switch r.Code {
	case NotClassified:
		return []Part{{Kind: PartLiteral, Text: "not classified"}}
	case NoOffering:
		return []Part{
			{Kind: PartLiteral, Text: "no offering in the "},
			{Kind: PartTarget, Text: r.Target},
			{Kind: PartLiteral, Text: " catalog for "},
			{Kind: PartCategory, Text: string(r.Category)},
		}
	case AllCapabilities:
		return []Part{
			{Kind: PartOffering, Text: r.Offering},
			{Kind: PartLiteral, Text: " offers every capability the resource uses"},
		}
	case MissingCapability:
		return []Part{
			{Kind: PartOffering, Text: r.Offering},
			{Kind: PartLiteral, Text: " does not offer "},
			{Kind: PartCapability, Text: string(r.Capability)},
		}
	case NotOfferedInRegion:
		return []Part{
			{Kind: PartLiteral, Text: "not offered in "},
			{Kind: PartRegion, Text: r.Region},
		}
	default:
		return []Part{{Kind: PartLiteral, Text: string(r.Code)}}
	}
}

// String returns the reason's sentence: the texts of Parts, concatenated. It is
// plain text for logs, errors and JSON; it is not safe for Markdown or HTML.
// Renderers use Parts and escape each part for their format.
func (r Reason) String() string {
	var b strings.Builder

	for _, p := range r.Parts() {
		b.WriteString(p.Text)
	}

	return b.String()
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
	// first: Direct before Partial, Partial candidates missing fewer capabilities
	// first, remaining ties by offering name. It is nil when there is none.
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
// Partial, Partial candidates by how many capabilities they lack (fewest first),
// and remaining ties by offering name in byte order. The result's fit and
// reasons are those of the best candidate: one AllCapabilities reason for
// Direct, one MissingCapability reason per lacking capability for Partial.
//
// Regions are not compared: where a resource runs today says nothing about which
// target region it must move to. Map only reads r and c.
//
// Map expects r to come from an inventory.Inventory and c from catalog.New,
// which validate and normalise them; a Resource or Catalog built any other way
// is not validated here.
func Map(r inventory.Resource, c catalog.Catalog) Result {
	return MapIn(r, c, "")
}

// MapIn matches resource r against the offerings of catalog c that are
// available in region (catalog.Offering.AvailableIn, compared exactly).
//
// With region "" it returns exactly what Map returns: availability is judged
// across all of the target's regions. Otherwise an offering that serves the
// resource's category but is not available in region is not a candidate. A
// resource whose category some offering serves, but none in region, has fit
// None with one NotOfferedInRegion reason (Target, Category and Region set). A
// resource whose category no offering serves in any region keeps the reason
// NoOffering, and an unclassified one NotClassified, as with Map. A resource
// with a candidate in region gets the fit, candidates and reasons Map gives it
// from those candidates alone.
//
// MapIn does not check that region is one of the target's regions: a region no
// offering lists gives NotOfferedInRegion for every resource with a category
// the catalog serves. Refusing an unknown region is the application layer's
// job (analysis.ErrUnknownRegion).
func MapIn(r inventory.Resource, c catalog.Catalog, region string) Result {
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

	if region != "" {
		offerings = slices.DeleteFunc(offerings, func(o catalog.Offering) bool { return !o.AvailableIn(region) })
		if len(offerings) == 0 {
			result.Reasons = []Reason{{
				Code:     NotOfferedInRegion,
				Target:   c.Target(),
				Category: r.Category,
				Region:   region,
			}}

			return result
		}
	}

	candidates := make([]Candidate, 0, len(offerings))
	for _, o := range offerings {
		candidates = append(candidates, candidate(r, o))
	}

	slices.SortStableFunc(candidates, func(a, b Candidate) int {
		if d := a.Fit.rank() - b.Fit.rank(); d != 0 {
			return d
		}

		if d := len(a.Missing) - len(b.Missing); d != 0 {
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
