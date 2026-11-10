package analysis

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

var (
	// ErrNilPort is returned by New when a port is nil.
	ErrNilPort = errors.New("nil port")
	// ErrInvalidReportMeta is returned by Run for report metadata a report cannot
	// carry safely, such as an inventory name that is not valid free text or is a
	// path rather than a base name.
	ErrInvalidReportMeta = errors.New("invalid report metadata")
)

// Analysis is the use case that assesses one source inventory against one
// target catalog and reports the result. Build it with New.
type Analysis struct {
	inventory InventorySource
	catalog   CatalogSource
	reporter  Reporter
}

// New returns an Analysis that reads from inv and cat and renders through rep.
// Every port is required: the error for a nil one wraps ErrNilPort.
func New(inv InventorySource, cat CatalogSource, rep Reporter) (*Analysis, error) {
	switch {
	case inv == nil:
		return nil, fmt.Errorf("%w: inventory source", ErrNilPort)
	case cat == nil:
		return nil, fmt.Errorf("%w: catalog source", ErrNilPort)
	case rep == nil:
		return nil, fmt.Errorf("%w: reporter", ErrNilPort)
	}

	return &Analysis{inventory: inv, catalog: cat, reporter: rep}, nil
}

// Run loads the inventory, loads the catalog, assesses the one against the other
// and renders the assessment with meta to w.
//
// meta is checked before any port is called; the error for invalid metadata
// wraps ErrInvalidReportMeta, and also text.ErrInvalid when a value is not valid
// free text. An error from a port stops the run and is wrapped with the stage
// that failed: "load inventory", "load catalog" or "render report".
//
// Run checks ctx itself before the first port is called and again before
// rendering, so a cancelled run never loads an inventory it will not report and
// never starts a report, even when an adapter ignores ctx. The error for either
// wraps ctx.Err() and names the point the run stopped at, never a stage that did
// not run.
//
// Blockers are a result, not an error: an assessment in which nothing can move
// is rendered like any other.
func (a *Analysis) Run(ctx context.Context, w io.Writer, meta ReportMeta) error {
	if err := meta.validate(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelled before loading the inventory: %w", err)
	}

	inv, err := a.inventory.Inventory(ctx)
	if err != nil {
		return fmt.Errorf("load inventory: %w", err)
	}

	cat, err := a.catalog.Catalog(ctx)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}

	result := assessment.Assess(inv, cat)

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelled before rendering the report: %w", err)
	}

	if err := a.reporter.Report(ctx, w, result, meta); err != nil {
		return fmt.Errorf("render report: %w", err)
	}

	return nil
}

// validate checks that every field of meta is valid free text and that
// InventoryName, when set, is a base name. The error names the field and shows
// its value through text.Quote.
func (m ReportMeta) validate() error {
	if err := text.Validate(m.InventoryName); err != nil {
		return fmt.Errorf("%w: inventory name %s: %w", ErrInvalidReportMeta, text.Quote(m.InventoryName), err)
	}

	if m.InventoryName != "" && !isBaseName(m.InventoryName) {
		return fmt.Errorf("%w: inventory name %s is a path, want the file's base name",
			ErrInvalidReportMeta, text.Quote(m.InventoryName))
	}

	return nil
}

// isBaseName reports whether name is a file's base name: it holds no path
// separator of any operating system, '/' or '\', and is not "." or "..". The
// rule is the same on every OS, so a report never shows a directory whatever
// platform built the name.
func isBaseName(name string) bool {
	return !strings.ContainsAny(name, `/\`) && name != "." && name != ".."
}
