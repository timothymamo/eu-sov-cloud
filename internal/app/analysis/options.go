package analysis

import (
	"context"
	"errors"
	"io"
)

// ErrUnknownRegion is wrapped by a *RegionError: a region was chosen that the
// target's catalog does not list.
var ErrUnknownRegion = errors.New("unknown region")

// RegionError is returned by RunWith when a chosen region is not one of the
// target's regions (catalog.Catalog.Regions). It is a usage error: the driving
// adapter shows it with the target's regions (PRD #190 milestone 8 makes it the
// CLI's exit 2). errors.Is(err, ErrUnknownRegion) holds for it.
type RegionError struct {
	// Target is the key of the target in a Comparison, and "" for an Analysis.
	Target string
	// Region is the region that was chosen, as given.
	Region string
	// Regions are the regions the target lists, sorted, as
	// catalog.Catalog.Regions returns them.
	Regions []string
}

// Error is one line naming the target (when there is a key), the chosen region
// and the target's regions, every value shown through text.Quote, for example
// `target "scaleway": region "fr-paris" is not one of the target's regions:
// "fr-par", "nl-ams"`. The exact text may vary.
func (e *RegionError) Error() string {
	// TODO(PRD #190 M6): not implemented yet.
	return ""
}

// Unwrap returns ErrUnknownRegion.
func (e *RegionError) Unwrap() error {
	// TODO(PRD #190 M6): not implemented yet.
	return nil
}

// Options are the choices of a single-target run (PRD #190). The zero Options
// is what Run does.
type Options struct {
	// Region is the one target region to assess and estimate in, and "" for
	// every region (availability across all of them, estimates in the snapshot's
	// default region). A region the catalog does not list is a *RegionError.
	Region string
	// Estimate turns plan matching and estimates on.
	Estimate bool
}

// RunWith runs the analysis as Run does, with opts. After the catalog is
// loaded, and before anything is assessed or rendered, a non-empty opts.Region
// is checked against the catalog's regions (exact comparison); one it does not
// list stops the run with a *RegionError (Target ""), returned unwrapped or
// wrapped so that errors.As and errors.Is(err, ErrUnknownRegion) find it, and
// the reporter is not called. The assessment is
// assessment.AssessWith(inv, cat, assessment.Options{Region: opts.Region,
// Estimate: opts.Estimate}).
//
// Run(ctx, w, meta) is RunWith(ctx, w, meta, Options{}).
func (a *Analysis) RunWith(ctx context.Context, w io.Writer, meta ReportMeta, opts Options) error {
	// TODO(PRD #190 M6): not implemented yet.
	_, _, _, _ = ctx, w, meta, opts

	return errNotImplemented
}

// ComparisonOptions are the choices of a comparison run that apply to every
// target (PRD #190). The region is per target: Target.Region. The zero
// ComparisonOptions is what Run does.
type ComparisonOptions struct {
	// Estimate turns plan matching and estimates on for every target.
	Estimate bool
}

// RunWith runs the comparison as Run does, with opts. Each target is assessed
// with assessment.AssessWith(inv, cat, assessment.Options{Region:
// target.Region, Estimate: opts.Estimate}). After a target's catalog is loaded,
// a non-empty Target.Region is checked against that catalog's regions; one it
// does not list stops the run with a *RegionError whose Target is the target's
// key, found by errors.As and errors.Is(err, ErrUnknownRegion), and nothing is
// rendered.
//
// Run(ctx, w, meta) is RunWith(ctx, w, meta, ComparisonOptions{}), which still
// honours each Target.Region.
func (c *Comparison) RunWith(ctx context.Context, w io.Writer, meta ReportMeta, opts ComparisonOptions) error {
	// TODO(PRD #190 M6): not implemented yet.
	_, _, _, _ = ctx, w, meta, opts

	return errNotImplemented
}

// errNotImplemented is what the stubs of PRD #190 milestone 6 return until they
// are implemented.
var errNotImplemented = errors.New("not implemented")
