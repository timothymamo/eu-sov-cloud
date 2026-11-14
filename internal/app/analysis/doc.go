// Package analysis is the application use case that runs an analysis end to
// end: check the report metadata, load the source inventory, load the target
// catalog, assess the one against the other and hand the assessment to a
// reporter.
//
// It declares the ports it consumes, next to the consumer: InventorySource,
// CatalogSource and Reporter, each taking a context. ReportMeta carries what a
// report shows besides the assessment (the inventory file's base name), and Run
// validates it before any port is called. The writer a report goes to is
// chosen by the driving adapter and passed through Run to the reporter.
//
// Comparison is the second use case: it reads the inventory once, assesses it
// against several targets' catalogs in key order, compares the assessments with
// assessment.Compare and hands the comparison to a ComparisonReporter. Analysis
// and Reporter stay unchanged; docs/adr/0016 records why the comparison has its
// own port.
//
// Both use cases take the choices of PRD #190 through RunWith: a region per
// target (Options.Region, Target.Region), which scopes availability and is the
// estimate region, and whether to estimate. A region the target's catalog does
// not list is a *RegionError wrapping ErrUnknownRegion, returned before
// anything is assessed (ADR 0027).
//
// The package depends only on internal/domain/... and the standard library.
// Concrete adapters implement the ports and are wired in cmd/shift2eu, never
// imported here. The port shapes are recorded in docs/adr/0005.
package analysis
