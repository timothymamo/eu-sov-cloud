package analysis

import (
	"context"
	"io"

	"github.com/timothymamo/shift2eu/internal/domain/assessment"
	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
)

// InventorySource is the port through which the analysis gets the inventory of
// the source being analysed: a file, or later a live, read-only discovery of a
// hyperscaler account.
//
// Inventory returns an inventory built by inventory.New. It returns the same
// inventory on every call for unchanged input, and an error wrapping ctx.Err()
// once ctx is done. The contract suite in internal/adapter/source/sourcetest
// pins this behaviour for every implementation.
//
// An error is a single line that names the input it failed on; values from the
// input or its location are shown through text.Quote; it never contains
// credentials, signed request URLs or raw response bodies.
type InventorySource interface {
	Inventory(ctx context.Context) (inventory.Inventory, error)
}

// CatalogSource is the port through which the analysis gets the catalog of the
// target being compared.
//
// Catalog returns a catalog built by catalog.New. It returns the same catalog
// on every call for unchanged input, and an error wrapping ctx.Err() once ctx is
// done. The contract suite in internal/adapter/target/targettest pins this
// behaviour for every implementation.
//
// An error is a single line that names the input it failed on; values from the
// input or its location are shown through text.Quote; it never contains
// credentials, signed request URLs or raw response bodies.
type CatalogSource interface {
	Catalog(ctx context.Context) (catalog.Catalog, error)
}

// ReportMeta is what a report says about the run besides the assessment itself.
type ReportMeta struct {
	// InventoryName is the base name of the inventory file, never its full path,
	// as the report shows it. It is empty when the inventory did not come from a
	// file. Run enforces the base-name rule: a name that holds '/' or '\', or is
	// "." or "..", is rejected on every OS. It must also be valid free text
	// (text.Validate). Run rejects an invalid name before any port is called.
	InventoryName string
}

// Reporter is the port that renders an assessment, for example as Markdown.
//
// Report writes the rendering of a and meta to w. Every value it takes from a
// or meta is untrusted and must be escaped for the output format. The driving
// adapter chooses w (standard output or a file) and owns it: Report neither
// opens nor closes it.
//
// Report may ignore ctx; Run checks the context before calling it. Output
// written before an error may be partial; the driving adapter decides what to
// do with it.
type Reporter interface {
	Report(ctx context.Context, w io.Writer, a assessment.Assessment, meta ReportMeta) error
}

// ComparisonReporter is the port that renders a comparison of several targets'
// assessments, for example as Markdown. It is separate from Reporter so that a
// reporter opts in to comparisons and the single-target path stays unchanged
// (docs/adr/0016).
//
// ReportComparison writes the rendering of c and meta to w. Every value it takes
// from c or meta is untrusted and must be escaped for the output format. The
// driving adapter chooses and owns w: ReportComparison neither opens nor closes
// it.
//
// ReportComparison may ignore ctx; Run checks the context before calling it.
// Output written before an error may be partial; the driving adapter decides
// what to do with it.
type ComparisonReporter interface {
	ReportComparison(ctx context.Context, w io.Writer, c assessment.Comparison, meta ReportMeta) error
}
