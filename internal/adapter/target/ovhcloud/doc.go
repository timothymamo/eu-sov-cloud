// Package ovhcloud describes the OVHcloud service catalog in domain terms. It
// is the driven adapter behind the analysis.CatalogSource port for the OVHcloud
// target, and it passes the contract suite in internal/adapter/target/targettest.
//
// The catalog is an embedded, dated snapshot, catalog.json (//go:embed),
// compiled by hand from OVHcloud's public regions page, product pages and
// documentation. The adapter makes no network calls and needs no credentials.
// SOURCES.md records where each offering, capability and region comes from, and
// the licence of each source.
//
// The snapshot covers OVHcloud Public Cloud only, in the regions OVHcloud has
// in EU member states, with one region code per location as the regions page
// prints it (PRD 11). A capability that OVHcloud offers in only some of a
// product's regions makes a separate offering, since an offering has one set
// of capabilities for all its regions: the 3-AZ variants, highly available
// across zones in the 3-AZ regions only, and Block Storage LUKS, the encrypted
// volumes not offered in every region, sit beside the product's main offering.
//
// catalog.json is a target catalog snapshot of schema version 1, the format
// every target with an embedded snapshot shares (ADR 0007). Source.Catalog and
// Regions read it through the shared reader, snapshot.Parse in
// internal/adapter/target/snapshot, which defines the format, its strictness
// and its errors; Regions returns the snapshot's declared EU region list. This
// package adds only the name of the embedded file to an error.
//
// To refresh the snapshot: edit catalog.json, update SOURCES.md and the
// retrievedOn date to match, run
//
//	go test ./internal/adapter/target/ovhcloud -update
//	go test ./internal/adapter/target/snapshot
//
// and review the diff of testdata/catalog.golden, which pins the parsed
// catalog, in the same change. The second run checks that every offering
// still has its own section in SOURCES.md. A change to the declared regions
// also changes the region list the package's tests write out, on purpose: it
// is checked against OVHcloud's regions page in review.
package ovhcloud
