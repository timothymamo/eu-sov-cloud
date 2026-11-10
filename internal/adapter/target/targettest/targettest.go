package targettest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// Expectations is what a target adapter declares about the catalog it must
// produce. Set every field: a check with nothing to compare against passes
// vacuously.
type Expectations struct {
	// Target is the target's display name the catalog must carry, for example
	// Scaleway.
	Target string
	// Regions lists every EU region the target's offerings may be available in.
	// An offering in any other region violates the contract.
	Regions []string
	// Categories lists the taxonomy categories the catalog must serve, each by at
	// least one offering.
	Categories []taxonomy.Category
}

// Check runs the target contract against src and returns every violation it
// finds, joined with errors.Join, or nil if src conforms. Each violation starts
// with the name of the check it breaks:
//
//   - catalog: Catalog returns without error;
//   - target: the catalog's Target is want.Target;
//   - snapshot: the catalog's snapshot has a source URL and a retrieval date;
//   - regions: every offering is available only in want.Regions;
//   - category: every category of want.Categories is served;
//   - repeat: a second call returns the same catalog as the first;
//   - cancel: a call with a cancelled context returns an error wrapping
//     context.Canceled (the violation wraps context.Canceled too).
func Check(ctx context.Context, src analysis.CatalogSource, want Expectations) error {
	var errs []error

	first, firstErr := src.Catalog(ctx)
	if firstErr != nil {
		errs = append(errs, fmt.Errorf("catalog: Catalog() returned error: %w", firstErr))
	} else {
		errs = append(errs, checkCatalog(first, want)...)
	}

	second, secondErr := src.Catalog(ctx)
	if secondErr != nil {
		errs = append(errs, fmt.Errorf("repeat: second Catalog() call returned error: %w", secondErr))
	} else if firstErr == nil {
		if d := diff(second, first); d != "" {
			errs = append(errs, fmt.Errorf("repeat: second Catalog() call differs from the first (want): %s", d))
		}
	}

	if err := checkCancel(ctx, src); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Run runs Check with the test's context and fails the test with every
// violation if src does not conform.
func Run(t *testing.T, src analysis.CatalogSource, want Expectations) {
	t.Helper()

	if err := Check(t.Context(), src, want); err != nil {
		t.Fatalf("target contract violated:\n%v", err)
	}
}

// checkCatalog returns the violations of the target, snapshot, regions and
// category checks by cat.
func checkCatalog(cat catalog.Catalog, want Expectations) []error {
	var errs []error

	if cat.Target() != want.Target {
		errs = append(errs, fmt.Errorf("target: Target() = %s, want %s", text.Quote(cat.Target()), text.Quote(want.Target)))
	}

	snapshot := cat.Snapshot()

	if snapshot.SourceURL == "" {
		errs = append(errs, errors.New("snapshot: SourceURL is empty, want the page or API the catalog was taken from"))
	}

	if snapshot.RetrievedOn.IsZero() {
		errs = append(errs, errors.New("snapshot: RetrievedOn is zero, want the date the catalog was retrieved"))
	}

	for _, o := range cat.Offerings() {
		for _, region := range o.Regions {
			if !slices.Contains(want.Regions, region) {
				errs = append(errs, fmt.Errorf("regions: offering %s is available in region %s, want one of %s",
					text.Quote(o.Name), text.Quote(region), quoteAll(want.Regions)))
			}
		}
	}

	for _, category := range want.Categories {
		if len(cat.OfferingsFor(category)) == 0 {
			errs = append(errs, fmt.Errorf("category: no offering serves %s", text.Quote(string(category))))
		}
	}

	return errs
}

// checkCancel calls src with an already-cancelled context and returns a
// violation wrapping context.Canceled unless src returns an error wrapping it.
func checkCancel(ctx context.Context, src analysis.CatalogSource) error {
	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	_, err := src.Catalog(cancelled)

	switch {
	case err == nil:
		return fmt.Errorf("cancel: Catalog() with a cancelled context returned no error, want one wrapping %w",
			context.Canceled)
	case !errors.Is(err, context.Canceled):
		return fmt.Errorf("cancel: Catalog() with a cancelled context returned error %w, want one wrapping %w",
			err, context.Canceled)
	default:
		return nil
	}
}

// diff describes the first difference between got and want, or returns "" if
// they are equal. Values are shown through text.Quote.
func diff(got, want catalog.Catalog) string {
	if got.Target() != want.Target() {
		return fmt.Sprintf("target %s, want %s", text.Quote(got.Target()), text.Quote(want.Target()))
	}

	gs, ws := got.Snapshot(), want.Snapshot()

	if gs.SourceURL != ws.SourceURL {
		return "snapshot source URLs differ"
	}

	if !gs.RetrievedOn.Equal(ws.RetrievedOn) {
		return fmt.Sprintf("snapshot retrieved on %s, want %s",
			gs.RetrievedOn.Format(time.RFC3339), ws.RetrievedOn.Format(time.RFC3339))
	}

	g, w := got.Offerings(), want.Offerings()

	for i := range max(len(g), len(w)) {
		switch {
		case i >= len(g):
			return fmt.Sprintf("offering %s missing, want %d offerings, got %d", text.Quote(w[i].Name), len(w), len(g))
		case i >= len(w):
			return fmt.Sprintf("unexpected offering %s, want %d offerings, got %d", text.Quote(g[i].Name), len(w), len(g))
		case !equalOffering(g[i], w[i]):
			return fmt.Sprintf("offerings[%d] = %s, want %s", i, describe(g[i]), describe(w[i]))
		}
	}

	return ""
}

// equalOffering reports whether a and b are the same offering, field by field.
func equalOffering(a, b catalog.Offering) bool {
	return a.Name == b.Name &&
		slices.Equal(a.Categories, b.Categories) &&
		slices.Equal(a.Capabilities, b.Capabilities) &&
		slices.Equal(a.Regions, b.Regions)
}

// describe formats o for a violation message, quoting every value.
func describe(o catalog.Offering) string {
	return fmt.Sprintf("{Name: %s, Categories: %s, Capabilities: %s, Regions: %s}",
		text.Quote(o.Name), quoteAll(o.Categories), quoteAll(o.Capabilities), quoteAll(o.Regions))
}

// quoteAll formats values as a bracketed list, each value through text.Quote.
func quoteAll[S ~string](values []S) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = text.Quote(string(v))
	}

	return fmt.Sprint(quoted)
}
