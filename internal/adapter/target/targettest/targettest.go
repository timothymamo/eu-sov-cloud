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
// produce. Every field is required; Check reports an expectations violation for
// an empty one.
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
//   - expectations: every field of want is set;
//   - catalog: Catalog returns without error;
//   - target: the catalog's Target is want.Target;
//   - snapshot: the catalog's snapshot has a source URL and a retrieval date;
//   - regions: every offering is available only in want.Regions (skipped
//     when want.Regions is empty, which is already an expectations violation);
//   - category: every category of want.Categories is served;
//   - repeat: a second call returns the same catalog as the first;
//   - cancel: a call with a cancelled context returns an error wrapping
//     context.Canceled (the violation wraps context.Canceled too).
//
// An error from src is shown through text.Quote and stays in the violation's
// chain.
func Check(ctx context.Context, src analysis.CatalogSource, want Expectations) error {
	errs := checkExpectations(want)

	first, firstErr := src.Catalog(ctx)
	if firstErr != nil {
		errs = append(errs, fmt.Errorf("catalog: Catalog() returned error: %w", adapterError{firstErr}))
	} else {
		errs = append(errs, checkCatalog(first, want)...)
	}

	second, secondErr := src.Catalog(ctx)
	if secondErr != nil {
		errs = append(errs, fmt.Errorf("repeat: second Catalog() call returned error: %w", adapterError{secondErr}))
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

// checkExpectations returns an expectations violation for every empty field
// of want, naming the field: a check with nothing to compare against would pass
// vacuously.
func checkExpectations(want Expectations) []error {
	var errs []error

	if want.Target == "" {
		errs = append(errs, errors.New("expectations: Target is empty, want the target's display name"))
	}

	if len(want.Regions) == 0 {
		errs = append(errs, errors.New("expectations: Regions is empty, want every EU region the offerings may be in"))
	}

	if len(want.Categories) == 0 {
		errs = append(errs, errors.New("expectations: Categories is empty, want every category the catalog must serve"))
	}

	return errs
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

	errs = append(errs, checkRegions(cat, want.Regions)...)

	for _, category := range want.Categories {
		if len(cat.OfferingsFor(category)) == 0 {
			errs = append(errs, fmt.Errorf("category: no offering serves %s", text.Quote(string(category))))
		}
	}

	return errs
}

// checkRegions returns a regions violation for every offering region of cat
// outside allowed. With no allowed regions it returns nothing: the empty
// expectation is already an expectations violation, and repeating it for every
// offering region would only bury it.
func checkRegions(cat catalog.Catalog, allowed []string) []error {
	if len(allowed) == 0 {
		return nil
	}

	var errs []error

	for _, o := range cat.Offerings() {
		for _, region := range o.Regions {
			if !slices.Contains(allowed, region) {
				errs = append(errs, fmt.Errorf("regions: offering %s is available in region %s, want one of %s",
					text.Quote(o.Name), text.Quote(region), quoteAll(allowed)))
			}
		}
	}

	return errs
}

// adapterError is an error returned by the adapter under test, as a violation
// shows it: Error quotes the adapter's message through text.Quote, so a newline
// or an escape sequence in it never reaches a terminal raw, and Unwrap keeps the
// adapter's error in the chain for errors.Is and errors.As.
type adapterError struct {
	err error
}

// Error returns the adapter's message through text.Quote.
func (e adapterError) Error() string {
	return text.Quote(e.err.Error())
}

// Unwrap returns the adapter's error.
func (e adapterError) Unwrap() error {
	return e.err
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
			adapterError{err}, context.Canceled)
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
