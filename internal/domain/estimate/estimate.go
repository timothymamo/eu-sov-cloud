package estimate

import (
	"github.com/timothymamo/shift2eu/internal/domain/mapping"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
)

// HoursPerMonth is the hours of an average month, 365 × 24 / 12: the hours a
// monthly list price stands for, and the hoursPerMonth a resource that gives
// none runs (ADR 0025).
const HoursPerMonth = 730

// Method is how a taxonomy category is priced.
type Method string

const (
	// ByPlan is a category priced by plan: compute.vm, compute.baremetal,
	// kubernetes.managed (its nodes), database.postgresql and database.mysql.
	ByPlan Method = "plan"
	// ByStorage is a category priced by stored GiB: storage.block and
	// storage.object.
	ByStorage Method = "storage"
	// ByUsage is a category priced by requests or messages, which this tool does
	// not estimate: serverless.function and messaging.queue.
	ByUsage Method = "usage"
	// NotPriced is a category that is never priced: taxonomy.Unclassified, and
	// any category this package does not know.
	NotPriced Method = "not-priced"
)

// PricingOf returns how category c is priced.
func PricingOf(c taxonomy.Category) Method {
	// TODO(PRD #190 M6): not implemented yet.
	_ = c

	return ""
}

// Reason says why a resource is not estimated.
type Reason string

// The reasons a resource is not estimated, checked in this order: the first
// that applies is the one an Estimate gives.
const (
	// NoFit is the reason for a resource with fit mapping.None, including one
	// that is unclassified or not offered in the chosen region.
	NoFit Reason = "no-fit"
	// UsagePriced is the reason for a resource whose category is priced by usage
	// (ByUsage), or never priced (NotPriced) while still having a fit.
	UsagePriced Reason = "usage-priced"
	// NoSize is the reason for a resource that does not give the size its
	// category is priced by: vcpus and memoryGiB for ByPlan (both needed),
	// storageGiB for ByStorage.
	NoSize Reason = "no-size"
	// NoPlan is the reason for a ByPlan resource whose best candidate offerings
	// list no plan at all.
	NoPlan Reason = "no-plan"
	// NoPlanLargeEnough is the reason for a ByPlan resource whose vCPUs or memory
	// no plan of the best candidate offerings covers. Estimate.Largest names the
	// largest plan.
	NoPlanLargeEnough Reason = "no-plan-large-enough"
	// NoPrice is the reason for a resource whose covering plans (ByPlan), or
	// whose best candidate offerings' storage (ByStorage), have no price in the
	// estimate region. For ByPlan, Estimate.Plan names the covering plan that
	// would have been chosen.
	NoPrice Reason = "no-price"
	// NoStoragePrice is the reason for a ByPlan resource with storage above
	// what its chosen plan includes, whose offering has no per-GiB storage price
	// in the estimate region. Estimate.Plan names the chosen plan.
	NoStoragePrice Reason = "no-storage-price"
)

// Plan is the plan an estimate chose, or names in a reason, as the catalog
// lists it.
type Plan struct {
	// Name is the plan's name.
	Name string
	// VCPUs is the plan's vCPU count.
	VCPUs int64
	// MemoryMilliGiB is the plan's memory in thousandths of a GiB.
	MemoryMilliGiB int64
	// IncludedStorageGiB is the storage the plan includes, in GiB.
	IncludedStorageGiB int64
	// Priced is true when the plan has a price in the estimate region. It is
	// false for the plan of Reason NoPrice, and may be false for Largest.
	Priced bool
	// Price is the plan's monthly price in the estimate region when Priced, and
	// zero otherwise. A price of 0 with Priced true is a free plan.
	Price Money
}

// Breakdown is what an estimate is made of. Its parts sum to Estimate.Monthly.
type Breakdown struct {
	// Plan is the plan price × count × hoursPerMonth / 730; zero for ByStorage.
	Plan Money
	// Storage is the storage the estimate prices × the per-GiB price, never
	// scaled by hours: for compute.vm, compute.baremetal and kubernetes.managed
	// the GiB above what the plan includes per instance or node, × count; for
	// database.postgresql and database.mysql the GiB of the service's data above
	// what the plan includes, once (not × count: the snapshots' database storage
	// prices and included storage are for the whole service); for storage.block
	// the volume's GiB × count; for storage.object the stored GiB.
	Storage Money
	// FixedFee is the offering's fixed fee in the estimate region × hoursPerMonth
	// / 730, once per resource and never × count (ADR 0026); zero when the
	// offering charges none.
	FixedFee Money
}

// Estimate is the plan and monthly list price of one resource at one target.
type Estimate struct {
	// Region is the target region the estimate is for, whether or not the
	// resource could be estimated.
	Region string
	// Estimated is true when Monthly holds the resource's estimate. When it is
	// false, Reason says why, and Monthly and Breakdown are zero: a resource that
	// is not estimated is never counted as 0 EUR.
	Estimated bool
	// Reason is why the resource is not estimated, and "" when it is.
	Reason Reason
	// Offering is the name of the offering the estimate is on: the chosen plan's
	// offering, Largest's offering for NoPlanLargeEnough, the offering whose
	// storage price was used for ByStorage, and "" otherwise.
	Offering string
	// Plan is the chosen plan: set when Estimated for ByPlan, and for NoPrice
	// and NoStoragePrice on ByPlan; nil otherwise.
	Plan *Plan
	// Largest is the largest plan of the best candidate offerings, set for
	// NoPlanLargeEnough only.
	Largest *Plan
	// Monthly is the estimated monthly list price, exact, excluding VAT and
	// discounts, set when Estimated.
	Monthly Money
	// Breakdown is what Monthly is made of, set when Estimated.
	Breakdown Breakdown
	// CountAssumed is true when the estimate priced a resource whose category
	// takes a count but that gives none, as one (ADR 0025). It is set only when
	// Estimated.
	CountAssumed bool
	// HoursAssumed is true when the estimate priced a ByPlan resource that gives
	// no hoursPerMonth as running 730 hours (ADR 0025). It is set only when
	// Estimated.
	HoursAssumed bool
}

// For estimates the resource of result, a mapping onto a target's catalog, in
// region, the estimate region (the region the user chose, or the snapshot's
// default region). See the package documentation and ADR 0027 for the rules.
//
// For only reads result. It expects result to come from mapping.Map or
// mapping.MapIn on a resource from an inventory.Inventory and a catalog from
// catalog.New.
func For(result mapping.Result, region string) Estimate {
	// TODO(PRD #190 M6): not implemented yet.
	_, _ = result, region

	return Estimate{}
}

// Total sums the estimates of an assessment.
type Total struct {
	// Monthly is the exact sum of Monthly over the estimated resources only.
	Monthly Money
	// Estimated is how many resources were estimated.
	Estimated int
	// Resources is how many resources there are, estimated or not: a report
	// shows "Monthly for Estimated of Resources resources".
	Resources int
}

// Sum totals estimates: Monthly over those with Estimated true, how many those
// are, and len(estimates).
func Sum(estimates []Estimate) Total {
	// TODO(PRD #190 M6): not implemented yet.
	_ = estimates

	return Total{}
}
