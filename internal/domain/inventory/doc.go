// Package inventory models what is running today in a source hyperscaler
// (AWS, Azure, GCP) as a normalised, provider-neutral inventory: resources,
// their taxonomy category, region, size, dependencies and the provider-specific
// features they rely on.
//
// Domain layer: standard library only, no I/O.
package inventory
