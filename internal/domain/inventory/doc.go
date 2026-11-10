// Package inventory models what is running today in a source hyperscaler
// (AWS, Azure, GCP) as a normalised, provider-neutral inventory: resources,
// their taxonomy category, region, the capabilities they rely on, and the raw
// provider type for traceability.
//
// An Inventory is validated when it is built: unique IDs, known categories and
// capabilities. Its resources are kept in ID order so everything derived from it
// is deterministic.
//
// Domain layer: standard library only, no I/O.
package inventory
