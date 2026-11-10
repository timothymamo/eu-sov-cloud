package inventory_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
)

// newInventory is the only way to build an Inventory, so every test goes through it.
func newInventory(t *testing.T, source string, resources []inventory.Resource) inventory.Inventory {
	t.Helper()

	inv, err := inventory.New(source, resources)
	if err != nil {
		t.Fatalf("New(%q, %d resources) returned unexpected error: %v", source, len(resources), err)
	}

	return inv
}

func resourceIDs(resources []inventory.Resource) []string {
	ids := make([]string, 0, len(resources))
	for _, r := range resources {
		ids = append(ids, r.ID)
	}

	return ids
}

func TestNewAndAccessors(t *testing.T) {
	t.Parallel()

	resources := []inventory.Resource{
		{
			ID:           "db-example",
			RawType:      "AWS::RDS::DBInstance",
			Category:     taxonomy.DatabasePostgreSQL,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.PointInTimeRestore, taxonomy.ReadReplicas},
		},
		{
			ID:           "i-0123456789abcdef0",
			RawType:      "AWS::EC2::Instance",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
		},
	}

	inv := newInventory(t, "aws", resources)

	if got := inv.Source(); got != "aws" {
		t.Errorf("Source() = %q, want %q", got, "aws")
	}

	// db-example sorts before i-0123456789abcdef0 even though it was listed second.
	want := []inventory.Resource{resources[0], resources[1]}
	if diff := cmp.Diff(want, inv.Resources()); diff != "" {
		t.Errorf("Resources() mismatch (-want +got):\n%s", diff)
	}
}

func TestResourcesSortedByID(t *testing.T) {
	t.Parallel()

	// Deliberately not in ID order.
	resources := []inventory.Resource{
		{ID: "vol-0123456789abcdef0", RawType: "AWS::EC2::Volume", Category: taxonomy.StorageBlock, Region: "eu-west-1"},
		{ID: "i-0123456789abcdef0", RawType: "AWS::EC2::Instance", Category: taxonomy.ComputeVM, Region: "eu-west-1"},
		{ID: "db-example", RawType: "AWS::RDS::DBInstance", Category: taxonomy.DatabasePostgreSQL, Region: "eu-west-1"},
	}

	inv := newInventory(t, "aws", resources)

	want := []string{"db-example", "i-0123456789abcdef0", "vol-0123456789abcdef0"}
	if diff := cmp.Diff(want, resourceIDs(inv.Resources())); diff != "" {
		t.Errorf("Resources() IDs are not sorted ascending (-want +got):\n%s", diff)
	}
}

func TestResourcesDoesNotAliasInput(t *testing.T) {
	t.Parallel()

	// A fresh copy of the expected inventory, so mutating the input cannot change it.
	wantResources := func() []inventory.Resource {
		return []inventory.Resource{{
			ID:           "db-example",
			RawType:      "AWS::RDS::DBInstance",
			Category:     taxonomy.DatabasePostgreSQL,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.PointInTimeRestore, taxonomy.ReadReplicas},
		}}
	}

	input := wantResources()
	inv := newInventory(t, "aws", input)

	got := inv.Resources()
	if diff := cmp.Diff(wantResources(), got); diff != "" {
		t.Fatalf("Resources() mismatch (-want +got):\n%s", diff)
	}

	// Mutating the returned resources, including their capabilities, must not reach the inventory.
	got[0].ID = "tampered-example"
	got[0].Category = taxonomy.ComputeVM
	got[0].Capabilities[0] = taxonomy.Capability("tampered")

	if diff := cmp.Diff(wantResources(), inv.Resources()); diff != "" {
		t.Errorf("Resources() changed after mutating a previous result (-want +got):\n%s", diff)
	}

	// Mutating the slice handed to New must not reach the inventory either.
	input[0].ID = "tampered-example"
	input[0].Capabilities[1] = taxonomy.Capability("tampered")
	input = append(input, inventory.Resource{ID: "extra-example", Category: taxonomy.StorageObject})

	if diff := cmp.Diff(wantResources(), inv.Resources()); diff != "" {
		t.Errorf("Resources() changed after mutating the input slice (-want +got):\n%s", diff)
	}
}

// An explicitly unclassified resource is a fact about the source, not an error: it
// must survive New and be reported (with fit none) later.
func TestNewKeepsUnclassifiedResource(t *testing.T) {
	t.Parallel()

	inv := newInventory(t, "aws", []inventory.Resource{{
		ID:       "mystery-example",
		RawType:  "AWS::Unknown::Thing",
		Category: taxonomy.Unclassified,
		Region:   "eu-west-1",
	}})

	got := inv.Resources()
	if diff := cmp.Diff([]string{"mystery-example"}, resourceIDs(got)); diff != "" {
		t.Fatalf("unclassified resource was dropped (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(taxonomy.Unclassified, got[0].Category); diff != "" {
		t.Errorf("category mismatch (-want +got):\n%s", diff)
	}
}

func TestNewAllowsResourceWithoutCapabilities(t *testing.T) {
	t.Parallel()

	inv := newInventory(t, "aws", []inventory.Resource{{
		ID:       "i-0123456789abcdef0",
		RawType:  "AWS::EC2::Instance",
		Category: taxonomy.ComputeVM,
		Region:   "eu-west-1",
	}})

	got := inv.Resources()
	if len(got) != 1 {
		t.Fatalf("Resources() returned %d resources, want 1", len(got))
	}

	if n := len(got[0].Capabilities); n != 0 {
		t.Errorf("len(Capabilities) = %d, want 0 (nil capabilities are valid)", n)
	}
}

func TestNewEmptyInventory(t *testing.T) {
	t.Parallel()

	inv := newInventory(t, "aws", nil)

	if got := inv.Source(); got != "aws" {
		t.Errorf("Source() = %q, want %q", got, "aws")
	}

	if n := len(inv.Resources()); n != 0 {
		t.Errorf("len(Resources()) = %d, want 0", n)
	}
}

func TestNewDeduplicatesAndSortsCapabilities(t *testing.T) {
	t.Parallel()

	inv := newInventory(t, "aws", []inventory.Resource{{
		ID:       "db-example",
		RawType:  "AWS::RDS::DBInstance",
		Category: taxonomy.DatabasePostgreSQL,
		Region:   "eu-west-1",
		Capabilities: []taxonomy.Capability{
			taxonomy.ReadReplicas,
			taxonomy.PointInTimeRestore,
			taxonomy.ReadReplicas,
		},
	}})

	want := []taxonomy.Capability{taxonomy.PointInTimeRestore, taxonomy.ReadReplicas}
	if diff := cmp.Diff(want, inv.Resources()[0].Capabilities); diff != "" {
		t.Errorf("Capabilities() are not de-duplicated and sorted (-want +got):\n%s", diff)
	}
}

func TestNewErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		resources []inventory.Resource
		wantErr   error
		wantMsg   []string
	}{
		{
			name: "empty id",
			resources: []inventory.Resource{{
				RawType:  "AWS::EC2::Instance",
				Category: taxonomy.ComputeVM,
				Region:   "eu-west-1",
			}},
			wantErr: inventory.ErrEmptyID,
		},
		{
			name: "duplicate id",
			resources: []inventory.Resource{
				{
					ID:       "i-0123456789abcdef0",
					RawType:  "AWS::EC2::Instance",
					Category: taxonomy.ComputeVM,
					Region:   "eu-west-1",
				},
				{
					ID:       "i-0123456789abcdef0",
					RawType:  "AWS::EC2::Instance",
					Category: taxonomy.ComputeVM,
					Region:   "eu-west-1",
				},
			},
			wantErr: inventory.ErrDuplicateID,
			wantMsg: []string{"i-0123456789abcdef0"},
		},
		{
			// A typo must be an error naming the resource, never an implicit unclassified.
			name: "unknown category",
			resources: []inventory.Resource{{
				ID:       "i-0123456789abcdef0",
				RawType:  "AWS::EC2::Instance",
				Category: taxonomy.Category("compute.mv"),
				Region:   "eu-west-1",
			}},
			wantErr: taxonomy.ErrUnknownCategory,
			wantMsg: []string{"i-0123456789abcdef0", "compute.mv"},
		},
		{
			name: "unknown capability",
			resources: []inventory.Resource{{
				ID:           "db-example",
				RawType:      "AWS::RDS::DBInstance",
				Category:     taxonomy.DatabasePostgreSQL,
				Region:       "eu-west-1",
				Capabilities: []taxonomy.Capability{taxonomy.Capability("pitr")},
			}},
			wantErr: taxonomy.ErrUnknownCapability,
			wantMsg: []string{"db-example", "pitr"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := inventory.New("aws", tc.resources)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("New() error = %v, want an error wrapping %v", err, tc.wantErr)
			}

			for _, want := range tc.wantMsg {
				if msg := err.Error(); !strings.Contains(msg, want) {
					t.Errorf("New() error = %q, want it to mention %q", msg, want)
				}
			}
		})
	}
}
