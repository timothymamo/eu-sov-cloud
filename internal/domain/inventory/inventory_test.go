package inventory_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
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
		source    string
		resources []inventory.Resource
		wantErr   error
		wantMsg   []string
	}{
		{
			name:      "empty source",
			source:    "",
			resources: []inventory.Resource{{ID: "i-0123456789abcdef0", Category: taxonomy.ComputeVM, Region: "eu-west-1"}},
			wantErr:   inventory.ErrEmptySource,
		},
		{
			name:      "whitespace only source",
			source:    "   ",
			resources: []inventory.Resource{{ID: "i-0123456789abcdef0", Category: taxonomy.ComputeVM, Region: "eu-west-1"}},
			wantErr:   inventory.ErrEmptySource,
		},
		{
			name:      "empty id",
			source:    "aws",
			resources: []inventory.Resource{{RawType: "AWS::EC2::Instance", Category: taxonomy.ComputeVM, Region: "eu-west-1"}},
			wantErr:   inventory.ErrEmptyID,
		},
		{
			name:      "whitespace only id",
			source:    "aws",
			resources: []inventory.Resource{{ID: "   ", RawType: "AWS::EC2::Instance", Category: taxonomy.ComputeVM, Region: "eu-west-1"}},
			wantErr:   inventory.ErrEmptyID,
		},
		{
			name:   "duplicate id",
			source: "aws",
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
			name:   "unknown category",
			source: "aws",
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
			name:   "unknown capability",
			source: "aws",
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

			_, err := inventory.New(tc.source, tc.resources)
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

// namesPosition reports whether msg points at the resource at index i. Before a
// resource has a valid ID the only way to name it is its position, which the
// package writes as resources[i]; a bare "resource i" is accepted too.
func namesPosition(msg string, i int) bool {
	n := strconv.Itoa(i)

	return strings.Contains(msg, "["+n+"]") || strings.Contains(msg, "resource "+n)
}

// Free text from a file ends up in a report and a terminal, so control
// characters, bidi overrides, invalid UTF-8 and over-long values are rejected
// instead of rendered. The error names the field and the resource it came from.
func TestNewRejectsInvalidText(t *testing.T) {
	t.Parallel()

	invalid := []struct {
		name  string
		value string
	}{
		{name: "ansi escape", value: "\x1b[2J"},
		{name: "newline", value: "row\nvalue"},
		{name: "pipe and newline", value: "|x\nrow"},
		{name: "right to left override", value: "a\u202eb"},
		{name: "zero width space", value: "a\u200bb"},
		{name: "invalid utf8", value: "a\xffb"},
		{name: "longer than max len", value: strings.Repeat("a", text.MaxLen+1)},
	}

	base := inventory.Resource{
		ID:       "i-0123456789abcdef0",
		RawType:  "AWS::EC2::Instance",
		Category: taxonomy.ComputeVM,
		Region:   "eu-west-1",
	}

	type testCase struct {
		name      string
		source    string
		resources []inventory.Resource
		// wantField is the name of the field the error must mention.
		wantField string
		// wantID is the resource the error must name, when it has a usable ID.
		wantID string
		// wantPos is the index the error must name when the ID itself is broken.
		wantPos int
	}

	tests := make([]testCase, 0, len(invalid)*4)

	for _, tc := range invalid {
		// A resource without a valid ID is named by its position (index 1 here).
		withBadID := base
		withBadID.ID = tc.value
		tests = append(tests, testCase{
			name:      "id/" + tc.name,
			source:    "aws",
			resources: []inventory.Resource{base, withBadID},
			wantField: "id",
			wantPos:   1,
		})

		withBadRawType := base
		withBadRawType.ID = "db-example"
		withBadRawType.RawType = tc.value
		tests = append(tests, testCase{
			name:      "raw type/" + tc.name,
			source:    "aws",
			resources: []inventory.Resource{base, withBadRawType},
			wantField: "raw type",
			wantID:    "db-example",
		})

		withBadRegion := base
		withBadRegion.ID = "vol-example"
		withBadRegion.Category = taxonomy.StorageBlock
		withBadRegion.Region = tc.value
		tests = append(tests, testCase{
			name:      "region/" + tc.name,
			source:    "aws",
			resources: []inventory.Resource{base, withBadRegion},
			wantField: "region",
			wantID:    "vol-example",
		})

		tests = append(tests, testCase{
			name:      "source/" + tc.name,
			source:    tc.value,
			resources: []inventory.Resource{base},
			wantField: "source",
		})
	}

	// U+2028 and U+2029 are Unicode white space, so the blank checks of source and
	// of the ID hide them before text.Validate sees them. A raw type has no blank
	// check, and some renderers still treat a line separator as a line break, so it
	// is rejected exactly like a newline.
	withLineSeparatorRawType := base
	withLineSeparatorRawType.ID = "db-example"
	withLineSeparatorRawType.RawType = "AWS::RDS::DBInstance\u2028example"
	tests = append(tests, testCase{
		name:      "raw type/line separator",
		source:    "aws",
		resources: []inventory.Resource{base, withLineSeparatorRawType},
		wantField: "raw type",
		wantID:    "db-example",
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := inventory.New(tc.source, tc.resources)
			if !errors.Is(err, text.ErrInvalid) {
				t.Fatalf("New() error = %v, want an error wrapping text.ErrInvalid", err)
			}

			msg := err.Error()
			if lower := strings.ToLower(msg); !strings.Contains(lower, tc.wantField) {
				t.Errorf("New() error = %q, want it to name the field %q", msg, tc.wantField)
			}

			switch {
			case tc.wantID != "":
				if !strings.Contains(msg, tc.wantID) {
					t.Errorf("New() error = %q, want it to name the resource %q", msg, tc.wantID)
				}
			case tc.wantField != "source":
				if !namesPosition(msg, tc.wantPos) {
					t.Errorf("New() error = %q, want it to name the position %d", msg, tc.wantPos)
				}
			}
		})
	}
}

// A resource with no capabilities and one with an empty capability list are the
// same resource; callers must not have to care which spelling a file uses.
func TestNewCanonicalisesEmptyCapabilities(t *testing.T) {
	t.Parallel()

	base := inventory.Resource{
		ID:       "i-0123456789abcdef0",
		RawType:  "AWS::EC2::Instance",
		Category: taxonomy.ComputeVM,
		Region:   "eu-west-1",
	}

	withNil := base
	withNil.Capabilities = nil

	withEmpty := base
	withEmpty.Capabilities = []taxonomy.Capability{}

	nilInv := newInventory(t, "aws", []inventory.Resource{withNil})
	emptyInv := newInventory(t, "aws", []inventory.Resource{withEmpty})

	if got := nilInv.Resources()[0].Capabilities; got != nil {
		t.Errorf("Capabilities = %#v for nil input, want nil", got)
	}

	if got := emptyInv.Resources()[0].Capabilities; got != nil {
		t.Errorf("Capabilities = %#v for empty input, want nil", got)
	}

	if diff := cmp.Diff(nilInv.Resources(), emptyInv.Resources()); diff != "" {
		t.Errorf("nil and empty capabilities are not canonicalised to the same resource (-nil +empty):\n%s", diff)
	}
}

// A file can carry a megabyte-long ID. Whatever rule rejects it, the error must
// stay readable and must not echo the whole value back.
func TestNewBoundedErrorForHugeID(t *testing.T) {
	t.Parallel()

	// 8 MiB, and not valid text for its length either.
	hugeID := strings.Repeat("i-0123456789abcdef0", 1<<19)

	_, err := inventory.New("aws", []inventory.Resource{{
		ID:       hugeID,
		RawType:  "AWS::EC2::Instance",
		Category: taxonomy.Category("compute.mv"),
		Region:   "eu-west-1",
	}})
	if err == nil {
		t.Fatalf("New() with a %d-byte ID and an unknown category returned no error", len(hugeID))
	}

	if n := len(err.Error()); n >= 2048 {
		t.Errorf("len(error) = %d, want less than 2048", n)
	}
}
