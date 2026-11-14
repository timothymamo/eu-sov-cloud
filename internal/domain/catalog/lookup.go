package catalog

// This file holds the lookups an estimate makes in a catalog (PRD #190
// milestone 6, ADR 0027): the regions a target lists, whether an offering is
// available in a region, and the price of a plan or of a GiB of storage in a
// region. Every region is compared exactly, as FixedFee compares it: no case
// folding, no trimming.

// Regions returns every region the catalog's target lists, sorted and free of
// duplicates: the union of every offering's regions and the snapshot's default
// region. A region the snapshot declares but no offering is available in is not
// in the domain (ADR 0024) and so is not listed. The result is a fresh slice:
// changing it does not change the catalog.
func (c Catalog) Regions() []string {
	// TODO(PRD #190 M6): not implemented yet.
	return nil
}

// AvailableIn reports whether the offering is available in region, compared
// exactly with its regions. It is false for "".
func (o Offering) AvailableIn(region string) bool {
	// TODO(PRD #190 M6): not implemented yet.
	_ = region

	return false
}

// PlanPrice returns the monthly price of the offering's plan named plan in
// region, and true. plan is compared exactly with the plans' names. A price
// whose Regions is nil applies to every region of the offering. It returns 0
// and false when the offering has no plan of that name, region is not one of
// the offering's regions, or no price of the plan applies to region: a plan
// unpriced in a region is "not estimated" there, never free.
func (o Offering) PlanPrice(plan, region string) (Amount, bool) {
	// TODO(PRD #190 M6): not implemented yet.
	_, _ = plan, region

	return 0, false
}

// StoragePrice returns the monthly price of one GiB of the offering's storage
// in region, and true. A price whose Regions is nil applies to every region of
// the offering. It returns 0 and false when region is not one of the offering's
// regions or no storage price applies to it.
func (o Offering) StoragePrice(region string) (Amount, bool) {
	// TODO(PRD #190 M6): not implemented yet.
	_ = region

	return 0, false
}
