package sourcetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// Check runs the source contract against src and returns every violation it
// finds, joined with errors.Join, or nil if src conforms. Each violation starts
// with the name of the check it breaks:
//
//   - want: Inventory returns want, without error;
//   - repeat: a second call returns the same inventory as the first;
//   - cancel: a call with a cancelled context returns an error wrapping
//     context.Canceled (the violation wraps context.Canceled too).
//
// want is the inventory src must produce from its recorded input. Inventories
// are compared exactly: source, then every resource field by field.
func Check(ctx context.Context, src analysis.InventorySource, want inventory.Inventory) error {
	var errs []error

	first, firstErr := src.Inventory(ctx)
	if firstErr != nil {
		errs = append(errs, fmt.Errorf("want: Inventory() returned error: %w", firstErr))
	} else if d := diff(first, want); d != "" {
		errs = append(errs, fmt.Errorf("want: Inventory() differs from the wanted inventory: %s", d))
	}

	second, secondErr := src.Inventory(ctx)
	if secondErr != nil {
		errs = append(errs, fmt.Errorf("repeat: second Inventory() call returned error: %w", secondErr))
	} else if firstErr == nil {
		if d := diff(second, first); d != "" {
			errs = append(errs, fmt.Errorf("repeat: second Inventory() call differs from the first (want): %s", d))
		}
	}

	if err := checkCancel(ctx, src); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Run runs Check with the test's context and fails the test with every
// violation if src does not conform.
func Run(t *testing.T, src analysis.InventorySource, want inventory.Inventory) {
	t.Helper()

	if err := Check(t.Context(), src, want); err != nil {
		t.Fatalf("source contract violated:\n%v", err)
	}
}

// checkCancel calls src with an already-cancelled context and returns a
// violation wrapping context.Canceled unless src returns an error wrapping it.
func checkCancel(ctx context.Context, src analysis.InventorySource) error {
	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	_, err := src.Inventory(cancelled)

	switch {
	case err == nil:
		return fmt.Errorf("cancel: Inventory() with a cancelled context returned no error, want one wrapping %w",
			context.Canceled)
	case !errors.Is(err, context.Canceled):
		return fmt.Errorf("cancel: Inventory() with a cancelled context returned error %w, want one wrapping %w",
			err, context.Canceled)
	default:
		return nil
	}
}

// diff describes the first difference between got and want, or returns "" if
// they are equal. Values are shown through text.Quote.
func diff(got, want inventory.Inventory) string {
	if got.Source() != want.Source() {
		return fmt.Sprintf("source %s, want %s", text.Quote(got.Source()), text.Quote(want.Source()))
	}

	g, w := got.Resources(), want.Resources()

	for i := range max(len(g), len(w)) {
		switch {
		case i >= len(g):
			return fmt.Sprintf("resource %s missing, want %d resources, got %d", text.Quote(w[i].ID), len(w), len(g))
		case i >= len(w):
			return fmt.Sprintf("unexpected resource %s, want %d resources, got %d", text.Quote(g[i].ID), len(w), len(g))
		case !equalResource(g[i], w[i]):
			return fmt.Sprintf("resources[%d] = %s, want %s", i, describe(g[i]), describe(w[i]))
		}
	}

	return ""
}

// equalResource reports whether a and b are the same resource, field by field.
func equalResource(a, b inventory.Resource) bool {
	return a.ID == b.ID &&
		a.RawType == b.RawType &&
		a.Category == b.Category &&
		a.Region == b.Region &&
		slices.Equal(a.Capabilities, b.Capabilities)
}

// describe formats r for a violation message, quoting every value.
func describe(r inventory.Resource) string {
	capabilities := make([]string, len(r.Capabilities))
	for i, c := range r.Capabilities {
		capabilities[i] = text.Quote(string(c))
	}

	return fmt.Sprintf("{ID: %s, RawType: %s, Category: %s, Region: %s, Capabilities: %v}",
		text.Quote(r.ID), text.Quote(r.RawType), text.Quote(string(r.Category)), text.Quote(r.Region), capabilities)
}
