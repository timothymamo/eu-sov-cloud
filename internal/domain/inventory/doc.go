// Package inventory models what is running today in a source hyperscaler
// (AWS, Azure, GCP) as a normalised, provider-neutral inventory: resources,
// their taxonomy category, region, the capabilities they rely on, and the raw
// provider type for traceability.
//
// An Inventory is validated when it is built with New: a source that is not
// blank, unique IDs that are not blank, free text (source, IDs, raw types,
// regions) that passes text.Validate, and known categories and capabilities. A
// raw type or region may be empty (unknown raw type, global resource) but never
// starts or ends with white space. An ID has no space separator other than the
// ASCII space, and IDs are compared ignoring the length of runs of white space;
// look-alike letters and Unicode normalisation are not compared. Values are
// stored as given, never trimmed. Its
// resources are kept in ID order, each with its capabilities sorted and
// de-duplicated, so everything derived from it is deterministic. A zero
// Inventory has not been validated; later layers only receive inventories
// built by New.
//
// Domain layer: standard library and other domain packages only, no I/O, no
// clock.
package inventory
