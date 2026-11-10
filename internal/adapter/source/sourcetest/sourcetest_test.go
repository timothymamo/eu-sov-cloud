package sourcetest_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/sourcetest"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// newInventory builds the only kind of inventory a source adapter can return:
// one that went through inventory.New.
func newInventory(t *testing.T, source string, resources []inventory.Resource) inventory.Inventory {
	t.Helper()

	inv, err := inventory.New(source, resources)
	if err != nil {
		t.Fatalf("inventory.New() returned unexpected error: %v", err)
	}

	return inv
}

// wantInventory is the recorded inventory a conforming source returns.
func wantInventory(t *testing.T) inventory.Inventory {
	t.Helper()

	return newInventory(t, "aws", []inventory.Resource{
		{
			ID:           "bucket-example",
			RawType:      "AWS::S3::Bucket",
			Category:     taxonomy.StorageObject,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.ObjectVersioning},
		},
		{
			ID:       "i-0123456789abcdef0",
			RawType:  "AWS::EC2::Instance",
			Category: taxonomy.ComputeVM,
			Region:   "eu-west-1",
		},
	})
}

// otherInventory differs from wantInventory in its source, so the contract
// suite's equality check must notice.
func otherInventory(t *testing.T) inventory.Inventory {
	t.Helper()

	return newInventory(t, "azure", []inventory.Resource{
		{
			ID:       "vm-example",
			RawType:  "Microsoft.Compute/virtualMachines",
			Category: taxonomy.ComputeVM,
			Region:   "westeurope",
		},
	})
}

// thirdInventory differs from wantInventory and from otherInventory, for the fake
// that changes its answer on every call.
func thirdInventory(t *testing.T) inventory.Inventory {
	t.Helper()

	return newInventory(t, "gcp", []inventory.Resource{
		{
			ID:       "bucket-third",
			RawType:  "storage.googleapis.com/Bucket",
			Category: taxonomy.StorageObject,
			Region:   "europe-west1",
		},
	})
}

// wantVariant is wantInventory with resource i changed by mutate. It keeps the
// source and the number of resources, so the only violation Check can report is
// the changed resource itself.
func wantVariant(t *testing.T, i int, mutate func(*inventory.Resource)) inventory.Inventory {
	t.Helper()

	resources := wantInventory(t).Resources()
	mutate(&resources[i])

	return newInventory(t, "aws", resources)
}

// fakeSource is an in-memory InventorySource whose answers a test scripts. It
// returns err if set and inventories in call order, repeating the last one, and
// records how often it was called. Unless it ignores the context, it returns
// ctx.Err() once the context is done, like a well-behaved adapter.
type fakeSource struct {
	inventories   []inventory.Inventory
	err           error
	ignoreContext bool
	calls         int
}

func (f *fakeSource) Inventory(ctx context.Context) (inventory.Inventory, error) {
	f.calls++

	if !f.ignoreContext {
		if err := ctx.Err(); err != nil {
			return inventory.Inventory{}, err
		}
	}

	if f.err != nil {
		return inventory.Inventory{}, f.err
	}

	if len(f.inventories) == 0 {
		return inventory.Inventory{}, nil
	}

	i := min(f.calls-1, len(f.inventories)-1)

	return f.inventories[i], nil
}

// mentions reports whether err says at least one of keywords. The suite's exact
// wording is not part of the contract; that every violation names its check is.
func mentions(err error, keywords ...string) bool {
	msg := err.Error()

	return slices.ContainsFunc(keywords, func(keyword string) bool {
		return strings.Contains(msg, keyword)
	})
}

// errHostile is an adapter error a terminal must never receive raw: it
// holds a newline and an ANSI escape sequence that clears the screen.
var errHostile = errors.New("boom\n\x1b[2J")

// TestCheck pins the first checks of the source contract suite: a source returns
// the recorded inventory, the same one every time, and honours cancellation.
func TestCheck(t *testing.T) {
	t.Parallel()

	want := wantInventory(t)
	other := otherInventory(t)
	third := thirdInventory(t)

	// wantMentions is a list of alternatives: the violation must contain at
	// least one keyword of every group.
	tests := []struct {
		name         string
		src          *fakeSource
		wantErr      bool
		wantMentions [][]string
		wantCancel   bool
		// wantQuoted, when set, is an adapter error the violation must show
		// quoted: the message may contain neither its raw newline nor its escape.
		wantQuoted error
	}{
		{
			name: "conforming source",
			src:  &fakeSource{inventories: []inventory.Inventory{want}},
		},
		{
			name:         "returns a different inventory",
			src:          &fakeSource{inventories: []inventory.Inventory{other}},
			wantErr:      true,
			wantMentions: [][]string{{"want", "expect", "differ", "equal"}},
		},
		{
			// The source is a discovery fact like any other: the same resources
			// from the wrong source is still a broken contract.
			name:         "returns the wanted resources from another source",
			src:          &fakeSource{inventories: []inventory.Inventory{newInventory(t, "azure", want.Resources())}},
			wantErr:      true,
			wantMentions: [][]string{{"want", "source"}},
		},
		{
			name:         "returns a different inventory on every call",
			src:          &fakeSource{inventories: []inventory.Inventory{other, third}},
			wantErr:      true,
			wantMentions: [][]string{{"want", "expect", "differ", "equal"}, {"repeat", "consecutive", "same", "equal", "stable", "state"}},
		},
		{
			name:         "consumes its state between calls",
			src:          &fakeSource{inventories: []inventory.Inventory{want, other}},
			wantErr:      true,
			wantMentions: [][]string{{"repeat", "consecutive", "same", "equal", "stable", "state"}},
		},
		{
			name:         "ignores a cancelled context",
			src:          &fakeSource{inventories: []inventory.Inventory{want}, ignoreContext: true},
			wantErr:      true,
			wantMentions: [][]string{{"cancel", "context"}},
			wantCancel:   true,
		},
		// Every field of a resource carries a discovery fact. A source that gets
		// one of them wrong for one resource has still broken the contract, and a
		// suite that compared a subset would pass it.
		{
			name: "changes a resource id",
			src: &fakeSource{inventories: []inventory.Inventory{wantVariant(t, 0, func(r *inventory.Resource) {
				r.ID = "bucket-renamed"
			})}},
			wantErr:      true,
			wantMentions: [][]string{{"want"}},
		},
		{
			name: "changes a raw type",
			src: &fakeSource{inventories: []inventory.Inventory{wantVariant(t, 0, func(r *inventory.Resource) {
				r.RawType = "AWS::S3::AccessPoint"
			})}},
			wantErr:      true,
			wantMentions: [][]string{{"want"}},
		},
		{
			name: "changes a category",
			src: &fakeSource{inventories: []inventory.Inventory{wantVariant(t, 0, func(r *inventory.Resource) {
				r.Category = taxonomy.ComputeVM
			})}},
			wantErr:      true,
			wantMentions: [][]string{{"want"}},
		},
		{
			name: "changes a region",
			src: &fakeSource{inventories: []inventory.Inventory{wantVariant(t, 0, func(r *inventory.Resource) {
				r.Region = "eu-central-1"
			})}},
			wantErr:      true,
			wantMentions: [][]string{{"want"}},
		},
		{
			name: "changes a capability",
			src: &fakeSource{inventories: []inventory.Inventory{wantVariant(t, 0, func(r *inventory.Resource) {
				r.Capabilities = []taxonomy.Capability{taxonomy.ObjectVersioning, taxonomy.Snapshots}
			})}},
			wantErr:      true,
			wantMentions: [][]string{{"want"}},
		},
		{
			name:         "drops a resource",
			src:          &fakeSource{inventories: []inventory.Inventory{newInventory(t, "aws", want.Resources()[:1])}},
			wantErr:      true,
			wantMentions: [][]string{{"want"}},
		},
		{
			name: "adds an unexpected resource",
			src: &fakeSource{inventories: []inventory.Inventory{newInventory(t, "aws", append(want.Resources(),
				inventory.Resource{
					ID:       "extra-example",
					RawType:  "AWS::SNS::Topic",
					Category: taxonomy.Unclassified,
					Region:   "eu-west-1",
				}))}},
			wantErr:      true,
			wantMentions: [][]string{{"want"}},
		},
		{
			// An extra resource that sorts after every wanted one is not a
			// per-field mismatch: it can only be reported by the length check, so
			// a suite that compared resources pairwise would pass it.
			name: "adds an unexpected resource that sorts last",
			src: &fakeSource{inventories: []inventory.Inventory{newInventory(t, "aws", append(want.Resources(),
				inventory.Resource{
					ID:       "zz-extra-example",
					RawType:  "AWS::SNS::Topic",
					Category: taxonomy.Unclassified,
					Region:   "eu-west-1",
				}))}},
			wantErr:      true,
			wantMentions: [][]string{{"want"}},
		},
		// An adapter that fails has still broken the contract, and the violation
		// must say so under the check it breaks.
		{
			name:         "returns an error",
			src:          &fakeSource{err: errors.New("inventory read failed")},
			wantErr:      true,
			wantMentions: [][]string{{"want"}},
		},
		// An adapter error is untrusted text: it reaches a terminal through the
		// violation, so a newline or an escape sequence in it must be escaped, and
		// the error itself must stay in the chain for errors.Is.
		{
			name:         "returns an error that would corrupt a terminal",
			src:          &fakeSource{err: errHostile},
			wantErr:      true,
			wantMentions: [][]string{{"want"}},
			wantQuoted:   errHostile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			err := sourcetest.Check(ctx, tt.src, want)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Check() returned unexpected error: %v", err)
				}

				if tt.src.calls < 2 {
					t.Errorf("Check() called Inventory() %d times, want at least 2 to check stability", tt.src.calls)
				}

				return
			}

			if err == nil {
				t.Fatal("Check() returned nil, want an error naming the violated check")
			}

			for i, group := range tt.wantMentions {
				if !mentions(err, group...) {
					t.Errorf("Check() error = %q, want it to mention violation %d (one of %v)", err, i+1, group)
				}
			}

			if got := errors.Is(err, context.Canceled); got != tt.wantCancel {
				t.Errorf("Check() error wraps context.Canceled = %t, want %t (error: %v)", got, tt.wantCancel, err)
			}

			if tt.wantQuoted != nil {
				msg := err.Error()

				if strings.Contains(msg, "\x1b") {
					t.Errorf("Check() error = %q, want the adapter error's escape sequence escaped", msg)
				}

				// violations are joined with a newline, so an adapter error echoed
				// raw would show its own newline directly after its text.
				if strings.Contains(msg, "boom\n") {
					t.Errorf("Check() error = %q, want the adapter error's newline escaped", msg)
				}

				if quoted := text.Quote(tt.wantQuoted.Error()); !strings.Contains(msg, quoted) {
					t.Errorf("Check() error = %q, want it to contain the quoted adapter error %s", msg, quoted)
				}

				if !errors.Is(err, tt.wantQuoted) {
					t.Errorf("Check() error = %v, want it to wrap the adapter error %v", err, tt.wantQuoted)
				}
			}
		})
	}
}

// TestRun pins that a conforming source passes the suite.
func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("conforming source", func(t *testing.T) {
		t.Parallel()

		sourcetest.Run(t, &fakeSource{inventories: []inventory.Inventory{wantInventory(t)}}, wantInventory(t))
	})
}
