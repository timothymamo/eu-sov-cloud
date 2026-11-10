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
// The package depends only on internal/domain/... and the standard library.
// Concrete adapters implement the ports and are wired in cmd/eusov, never
// imported here. The port shapes are recorded in docs/adr/0005.
package analysis
