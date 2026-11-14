package analysis

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/timothymamo/shift2eu/internal/domain/assessment"
	"github.com/timothymamo/shift2eu/internal/domain/text"
)

// ErrInvalidTargets is returned by NewComparison for a list of targets it cannot
// compare: an empty list, or a key that is empty, repeated or not valid free
// text.
var ErrInvalidTargets = errors.New("invalid targets")

// Target is one target to compare: the key it was selected by and the source of
// its catalog.
type Target struct {
	// Key names the target in the comparison, such as "scaleway". It must be
	// non-empty valid free text (text.Validate) and unique among the targets.
	Key string
	// Catalog is the source of the target's catalog.
	Catalog CatalogSource
	// Region is the one region of this target to assess and estimate in, and ""
	// for every region (PRD #190). Comparison.RunWith checks it against the
	// target's catalog once loaded (*RegionError).
	Region string
}

// Comparison is the use case that assesses one source inventory against several
// target catalogs and reports the assessments side by side. Build it with
// NewComparison.
type Comparison struct {
	inventory InventorySource
	// targets are in key order.
	targets  []Target
	reporter ComparisonReporter
}

// NewComparison returns a Comparison that reads from inv and from every target's
// catalog and renders through rep. Every port is required: the error for a nil
// one wraps ErrNilPort and, for a catalog, names the target's key. targets must
// be a non-empty list of distinct, non-empty keys that are valid free text;
// otherwise the error wraps ErrInvalidTargets. NewComparison keeps its own copy
// of targets.
func NewComparison(inv InventorySource, targets []Target, rep ComparisonReporter) (*Comparison, error) {
	switch {
	case inv == nil:
		return nil, fmt.Errorf("%w: inventory source", ErrNilPort)
	case rep == nil:
		return nil, fmt.Errorf("%w: reporter", ErrNilPort)
	case len(targets) == 0:
		return nil, fmt.Errorf("%w: no targets", ErrInvalidTargets)
	}

	// sorted is a copy of targets in key order, built by insertion: the lists
	// are a handful of targets long, and a repeated key shows up as an equal
	// neighbour.
	sorted := make([]Target, 0, len(targets))

	for _, target := range targets {
		if err := validTarget(target); err != nil {
			return nil, err
		}

		i := 0
		for i < len(sorted) && sorted[i].Key < target.Key {
			i++
		}

		if i < len(sorted) && sorted[i].Key == target.Key {
			return nil, fmt.Errorf("%w: target %s given more than once", ErrInvalidTargets, text.Quote(target.Key))
		}

		sorted = append(sorted, Target{})
		copy(sorted[i+1:], sorted[i:])
		sorted[i] = target
	}

	return &Comparison{inventory: inv, targets: sorted, reporter: rep}, nil
}

// validTarget checks one target's key and catalog source.
func validTarget(target Target) error {
	switch err := text.Validate(target.Key); {
	case err != nil:
		return fmt.Errorf("%w: target key %s: %w", ErrInvalidTargets, text.Quote(target.Key), err)
	case target.Key == "":
		return fmt.Errorf("%w: empty target key", ErrInvalidTargets)
	case target.Catalog == nil:
		return fmt.Errorf("%w: catalog source of target %s", ErrNilPort, text.Quote(target.Key))
	}

	return nil
}

// Run loads the inventory once, loads each target's catalog in key order,
// assesses the inventory against each, compares the assessments with
// assessment.Compare and renders the comparison with meta to w.
//
// meta is checked as Analysis.Run checks it, before any port is called. An
// error from a port stops the run, nothing is rendered, and the error is
// wrapped with the stage that failed: "load inventory",
// `target "<key>": load catalog` (the key is quoted) or "render report".
//
// Run checks ctx itself before the first port is called, before each target's
// catalog is loaded and again before rendering, so a cancelled run stops at
// the next target instead of loading the remaining catalogs.
//
// Blockers are a result, not an error.
func (c *Comparison) Run(ctx context.Context, w io.Writer, meta ReportMeta) error {
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

		assessed = append(assessed, assessment.TargetAssessment{
			Key:        target.Key,
			Assessment: assessment.Assess(inv, cat),
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
