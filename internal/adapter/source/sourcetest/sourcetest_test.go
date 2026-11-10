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

// fakeSource is an in-memory InventorySource whose answers a test scripts. It
// returns inventories in call order, repeating the last one, and records how
// often it was called. Unless it ignores the context, it returns ctx.Err() once
// the context is done, like a well-behaved adapter.
type fakeSource struct {
	inventories   []inventory.Inventory
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
