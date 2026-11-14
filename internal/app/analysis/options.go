package analysis

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/timothymamo/shift2eu/internal/domain/assessment"
	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/text"
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
	var b strings.Builder

	if e.Target != "" {
		b.WriteString("target ")
		b.WriteString(text.Quote(e.Target))
		b.WriteString(": ")
	}

	b.WriteString("region ")
	b.WriteString(text.Quote(e.Region))
	b.WriteString(" is not one of the target's regions: ")

	for i, region := range e.Regions {
		if i > 0 {
			b.WriteString(", ")
		}

		b.WriteString(text.Quote(region))
	}

	return b.String()
}

// Unwrap returns ErrUnknownRegion.
func (e *RegionError) Unwrap() error {
	return ErrUnknownRegion
}

// checkRegion returns a *RegionError for a non-empty region cat does not list,
// with key as its Target, and nil otherwise.
func checkRegion(key, region string, cat catalog.Catalog) error {
	if region == "" {
		return nil
	}

	regions := cat.Regions()
	for _, listed := range regions {
		if listed == region {
			return nil
		}
	}

	return &RegionError{Target: key, Region: region, Regions: regions}
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

	if err := checkRegion("", opts.Region, cat); err != nil {
		return err
	}

	result := assessment.AssessWith(inv, cat, assessment.Options{Region: opts.Region, Estimate: opts.Estimate})

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelled before rendering the report: %w", err)
	}

	if err := a.reporter.Report(ctx, w, result, meta); err != nil {
		return fmt.Errorf("render report: %w", err)
	}

	return nil
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
	if err := meta.validate(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelled before loading the inventory: %w", err)
	}

	inv, err := c.inventory.Inventory(ctx)
	if err != nil {
		return fmt.Errorf("load inventory: %w", err)
	}

	assessed := make([]assessment.TargetAssessment, 0, len(c.targets))

	for _, target := range c.targets {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("target %s: cancelled before loading the catalog: %w", text.Quote(target.Key), err)
		}

		cat, err := target.Catalog.Catalog(ctx)
		if err != nil {
			return fmt.Errorf("target %s: load catalog: %w", text.Quote(target.Key), err)
		}

		if err := checkRegion(target.Key, target.Region, cat); err != nil {
			return err
		}

		assessed = append(assessed, assessment.TargetAssessment{
			Key:        target.Key,
			Assessment: assessment.AssessWith(inv, cat, assessment.Options{Region: target.Region, Estimate: opts.Estimate}),
		})
	}

	comparison := assessment.Compare(assessed)

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelled before rendering the report: %w", err)
	}

	if err := c.reporter.ReportComparison(ctx, w, comparison, meta); err != nil {
		return fmt.Errorf("render report: %w", err)
	}

	return nil
}
