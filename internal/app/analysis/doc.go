// Package analysis is the application use case that runs an analysis end to
// end: load the source inventory, load the target catalogs, map, assess and
// hand the result to a reporter.
//
// It declares the ports (interfaces) it needs, such as an inventory source, a
// catalog source and a reporter, and depends only on internal/domain/... and
// those ports. Concrete adapters are wired in cmd/eusov, never imported here.
package analysis
