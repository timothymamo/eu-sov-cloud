// Package scaleway describes the Scaleway service catalog in domain terms. It
// is the driven adapter behind the analysis.CatalogSource port for the Scaleway
// target, and it passes the contract suite in internal/adapter/target/targettest.
//
// The catalog is an embedded, dated snapshot, catalog.json (//go:embed),
// compiled by hand from Scaleway's public documentation and SDK. The adapter
// makes no network calls and needs no credentials. SOURCES.md records where
// each offering, capability and region comes from.
//
// catalog.json is a JSON document of schema version 1: a header with the
// target's display name, the source URL and the retrieval date (retrievedOn,
// YYYY-MM-DD) of the data, the EU regions the snapshot declares, and the
// offerings, each with its taxonomy categories, capabilities and regions.
// Parse reads it strictly: an unknown field, an unsupported schema version, an
// offering in a region the header does not declare, or a value the taxonomy or
// catalog.New refuses is an error, never dropped or defaulted. Source.Catalog
// serves the embedded file through Parse, and Regions returns its declared
// region list.
//
// To refresh the snapshot: edit catalog.json, update SOURCES.md and the
// retrievedOn date to match, run
//
//	go test ./internal/adapter/target/scaleway -update
//
// and review the diff of testdata/catalog.golden, which pins the parsed
// catalog, in the same change. A change to the declared regions also changes
// the region list the package's tests write out, on purpose: it is checked
// against Scaleway's published regions in review.
package scaleway
