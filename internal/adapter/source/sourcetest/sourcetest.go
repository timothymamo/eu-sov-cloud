package sourcetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/timothymamo/shift2eu/internal/app/analysis"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/domain/text"
)

// Check runs the source contract against src and returns every violation it
// finds, joined with errors.Join, or nil if src conforms. Each violation starts
// with the name of the check it breaks:
//
//   - want: Inventory returns want, without error;
//   - repeat: a second call returns the same inventory as the first, even after
//     the first result was mutated (a source hands out copies, never memory it
//     keeps);
//   - targets: every relation of the inventory names a resource of the same
//     inventory;
//   - cancel: a call with a cancelled context returns an error wrapping
//     context.Canceled (the violation wraps context.Canceled too).
//
// want is the inventory src must produce from its recorded input. Inventories
// are compared exactly: source, then every resource field by field, the version 2
// size, usage, workload, relations and cost included. An error
// from src is shown through text.Quote and stays in the violation's chain.
func Check(ctx context.Context, src analysis.InventorySource, want inventory.Inventory) error {
	var errs []error

	// The wanted inventory is checked before anything is mutated below.
	if err := checkTargets(want); err != nil {
		errs = append(errs, err)
	}

	first, firstErr := src.Inventory(ctx)
	if firstErr != nil {
		errs = append(errs, fmt.Errorf("want: Inventory() returned error: %w", adapterError{firstErr}))
	} else if d := diff(first, want); d != "" {
		errs = append(errs, fmt.Errorf("want: Inventory() differs from the wanted inventory: %s", d))
	} else {
		// A source hands out copies: what a caller changes must not reach the next
		// call. The domain's Resources() copies, so this also pins that the source
		// did not keep and re-hand memory a caller can change.
		tamper(first.Resources())
	}

	second, secondErr := src.Inventory(ctx)
	if secondErr != nil {
		errs = append(errs, fmt.Errorf("repeat: second Inventory() call returned error: %w", adapterError{secondErr}))
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
			adapterError{err}, context.Canceled)
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
		slices.Equal(a.Capabilities, b.Capabilities) &&
		equalSize(a.Size, b.Size) &&
		equalUsage(a.Usage, b.Usage) &&
		a.Workload == b.Workload &&
		slices.Equal(a.Relations, b.Relations) &&
		equalCost(a.MonthlyCost, b.MonthlyCost)
}

// equalSize reports whether a and b are the same size, field by field, with a
// field that is not given different from one that is zero.
func equalSize(a, b *inventory.Size) bool {
	return compareNumber(valueOf(a, func(s *inventory.Size) *float64 { return s.VCPUs }),
		valueOf(b, func(s *inventory.Size) *float64 { return s.VCPUs })) &&
		compareNumber(valueOf(a, func(s *inventory.Size) *float64 { return s.MemoryGiB }),
			valueOf(b, func(s *inventory.Size) *float64 { return s.MemoryGiB })) &&
		compareNumber(valueOf(a, func(s *inventory.Size) *float64 { return s.StorageGiB }),
			valueOf(b, func(s *inventory.Size) *float64 { return s.StorageGiB })) &&
		compareNumber(valueOf(a, func(s *inventory.Size) *int { return s.Count }),
			valueOf(b, func(s *inventory.Size) *int { return s.Count }))
}

// equalUsage reports whether a and b are the same usage profile.
func equalUsage(a, b *inventory.Usage) bool {
	return compareNumber(valueOf(a, func(u *inventory.Usage) *float64 { return u.HoursPerMonth }),
		valueOf(b, func(u *inventory.Usage) *float64 { return u.HoursPerMonth }))
}

// equalCost reports whether a and b are the same cost: the same amount, given or
// not, and the same currency.
func equalCost(a, b *inventory.Cost) bool {
	if (a == nil) != (b == nil) {
		return false
	}

	if a == nil {
		return true
	}

	return compareNumber(a.Amount, b.Amount) && a.Currency == b.Currency
}

// valueOf returns the field fn names of s, or nil when s itself is not given.
func valueOf[T any, F any](s *T, fn func(*T) *F) *F {
	if s == nil {
		return nil
	}

	return fn(s)
}

// compareNumber reports whether a and b hold the same number, with nil, a field
// that is not given, different from any value.
func compareNumber[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}

// tamper changes every mutable field of resources, the copies a source handed
// out, so that a source which kept and re-handed that memory shows a different
// inventory on its next call.
func tamper(resources []inventory.Resource) {
	for i := range resources {
		r := &resources[i]
		r.ID = "tampered"
		r.RawType = "tampered"
		r.Region = "tampered"
		r.Workload = "tampered"
		r.Capabilities = append(r.Capabilities, taxonomy.Capability("tampered"))

		if r.Size != nil {
			setNumber(r.Size.VCPUs, -1)
			setNumber(r.Size.MemoryGiB, -1)
			setNumber(r.Size.StorageGiB, -1)
			setNumber(r.Size.Count, -1)
		}

		if r.Usage != nil {
			setNumber(r.Usage.HoursPerMonth, -1)
		}

		for j := range r.Relations {
			r.Relations[j].Target = "tampered"
		}

		if r.MonthlyCost != nil {
			setNumber(r.MonthlyCost.Amount, -1)
			r.MonthlyCost.Currency = "tampered"
		}
	}
}

// setNumber sets the value p points at to v, doing nothing when p is nil.
func setNumber[T any](p *T, v T) {
	if p != nil {
		*p = v
	}
}

// checkTargets returns a violation unless every relation of every resource of
// inv names a resource of the same inventory. The domain refuses such a relation
// already, so the check pins the property for an adapter that reaches the domain
// another way.
func checkTargets(inv inventory.Inventory) error {
	resources := inv.Resources()

	ids := make(map[string]struct{}, len(resources))
	for _, r := range resources {
		ids[r.ID] = struct{}{}
	}

	for _, r := range resources {
		for i, relation := range r.Relations {
			if _, ok := ids[relation.Target]; !ok {
				return fmt.Errorf("targets: relation %d of resource %s names %s, which is no resource of the inventory",
					i, text.Quote(r.ID), text.Quote(relation.Target))
			}
		}
	}

	return nil
}

// describe formats r for a violation message, quoting every value.
func describe(r inventory.Resource) string {
	capabilities := make([]string, len(r.Capabilities))
	for i, c := range r.Capabilities {
		capabilities[i] = text.Quote(string(c))
	}

	return fmt.Sprintf(
		"{ID: %s, RawType: %s, Category: %s, Region: %s, Capabilities: %v, Size: %s, Usage: %s, Workload: %s, Relations: %s, MonthlyCost: %s}",
		text.Quote(r.ID), text.Quote(r.RawType), text.Quote(string(r.Category)), text.Quote(r.Region), capabilities,
		describeSize(r.Size), describeUsage(r.Usage), text.Quote(r.Workload), describeRelations(r.Relations),
		describeCost(r.MonthlyCost))
}

// describeSize formats s, a size that may not be given, for a violation.
func describeSize(s *inventory.Size) string {
	if s == nil {
		return "not given"
	}

	return fmt.Sprintf("{vcpus: %s, memoryGiB: %s, storageGiB: %s, count: %s}",
		describeNumber(s.VCPUs), describeNumber(s.MemoryGiB), describeNumber(s.StorageGiB), describeNumber(s.Count))
}

// describeUsage formats u, a usage profile that may not be given.
func describeUsage(u *inventory.Usage) string {
	if u == nil {
		return "not given"
	}

	return fmt.Sprintf("{hoursPerMonth: %s}", describeNumber(u.HoursPerMonth))
}

// describeCost formats c, a cost that may not be given.
func describeCost(c *inventory.Cost) string {
	if c == nil {
		return "not given"
	}

	return fmt.Sprintf("{amount: %s, currency: %s}", describeNumber(c.Amount), text.Quote(c.Currency))
}

// describeRelations formats relations for a violation, quoting every value.
func describeRelations(relations []inventory.Relation) string {
	if len(relations) == 0 {
		return "none"
	}

	described := make([]string, len(relations))
	for i, relation := range relations {
		described[i] = text.Quote(string(relation.Type)) + " -> " + text.Quote(relation.Target)
	}

	return fmt.Sprint(described)
}

// describeNumber formats p, a number that may not be given.
func describeNumber[T any](p *T) string {
	if p == nil {
		return "not given"
	}

	return text.Quote(fmt.Sprint(*p))
}
