package analysis

import (
	"context"
	"io"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
)

// InventorySource is the port through which the analysis gets the inventory of
// the source being analysed: a file, or later a live, read-only discovery of a
// hyperscaler account.
//
// Inventory returns an inventory built by inventory.New. It returns the same
// inventory on every call for unchanged input, and an error wrapping ctx.Err()
// once ctx is done. The contract suite in internal/adapter/source/sourcetest
// pins this behaviour for every implementation.
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
type CatalogSource interface {
	Catalog(ctx context.Context) (catalog.Catalog, error)
}

// ReportMeta is what a report says about the run besides the assessment itself.
type ReportMeta struct {
	// InventoryName is the base name of the inventory file, never its full path,
	// as the report shows it. It is empty when the inventory did not come from a
	// file. It must be valid free text (text.Validate): Run rejects it otherwise,
	// before any port is called.
	InventoryName string
}

// Reporter is the port that renders an assessment, for example as Markdown.
//
// Report writes the rendering of a and meta to w. Every value it takes from a
// or meta is untrusted and must be escaped for the output format. The driving
// adapter chooses w (standard output or a file) and owns it: Report neither
// opens nor closes it. Report may have written part of its output when it
// returns an error.
type Reporter interface {
	Report(ctx context.Context, w io.Writer, a assessment.Assessment, meta ReportMeta) error
}
