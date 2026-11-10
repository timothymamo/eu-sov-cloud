package catalog_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
)

// validSnapshot is the dated, sourced snapshot every real catalog must carry.
func validSnapshot() catalog.Snapshot {
	return catalog.Snapshot{
		SourceURL:   "https://www.scaleway.com/en/pricing/",
		RetrievedOn: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}
}

// validOfferings is deliberately not in name order: Elastic Metal, Instances,
// Kapsule, Managed Database for PostgreSQL, Object Storage is the expected order.
func validOfferings() []catalog.Offering {
	return []catalog.Offering{
		{
			Name:         "Object Storage",
			Categories:   []taxonomy.Category{taxonomy.StorageObject},
			Capabilities: []taxonomy.Capability{taxonomy.ObjectVersioning, taxonomy.IPv6},
			Regions:      []string{"fr-par", "nl-ams", "pl-waw"},
		},
		{
			Name:         "Instances",
			Categories:   []taxonomy.Category{taxonomy.ComputeVM},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6, taxonomy.Autoscaling},
			Regions:      []string{"fr-par", "nl-ams", "pl-waw"},
		},
		{
			// One offering may serve several categories.
			Name:         "Elastic Metal",
			Categories:   []taxonomy.Category{taxonomy.ComputeVM, taxonomy.StorageBlock},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
			Regions:      []string{"fr-par", "nl-ams"},
		},
		{
			Name:         "Kapsule",
			Categories:   []taxonomy.Category{taxonomy.KubernetesManaged},
			Capabilities: []taxonomy.Capability{taxonomy.Autoscaling, taxonomy.HighAvailability},
			Regions:      []string{"fr-par", "nl-ams", "pl-waw"},
		},
		{
			Name:         "Managed Database for PostgreSQL",
			Categories:   []taxonomy.Category{taxonomy.DatabasePostgreSQL},
			Capabilities: []taxonomy.Capability{taxonomy.PointInTimeRestore, taxonomy.ReadReplicas},
			Regions:      []string{"fr-par", "nl-ams", "pl-waw"},
		},
	}
}

// offeringsSortedByName returns a fresh copy of validOfferings in name order.
func offeringsSortedByName() []catalog.Offering {
	offerings := validOfferings()
	slices.SortFunc(offerings, func(a, b catalog.Offering) int {
		return strings.Compare(a.Name, b.Name)
	})

	return offerings
}

func newCatalog(t *testing.T, target string, snapshot catalog.Snapshot, offerings []catalog.Offering) catalog.Catalog {
	t.Helper()

	cat, err := catalog.New(target, snapshot, offerings)
	if err != nil {
		t.Fatalf("New(%q, %d offerings) returned unexpected error: %v", target, len(offerings), err)
	}

	return cat
}

func offeringNames(offerings []catalog.Offering) []string {
	if len(offerings) == 0 {
		return nil
	}

	names := make([]string, 0, len(offerings))
	for _, o := range offerings {
		names = append(names, o.Name)
	}

	return names
}

func TestNewAndAccessors(t *testing.T) {
	t.Parallel()

	cat := newCatalog(t, "scaleway", validSnapshot(), validOfferings())

	if got := cat.Target(); got != "scaleway" {
		t.Errorf("Target() = %q, want %q", got, "scaleway")
	}

	if diff := cmp.Diff(validSnapshot(), cat.Snapshot()); diff != "" {
		t.Errorf("Snapshot() mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(offeringsSortedByName(), cat.Offerings()); diff != "" {
		t.Errorf("Offerings() are not sorted by name (-want +got):\n%s", diff)
	}
}

func TestOfferingsReturnsCopy(t *testing.T) {
	t.Parallel()

	cat := newCatalog(t, "scaleway", validSnapshot(), validOfferings())

	got := cat.Offerings()
	if len(got) == 0 {
		t.Fatal("Offerings() is empty")
	}

	got[0].Name = "Tampered"
	got[0].Categories[0] = taxonomy.Category("tampered")
	got[0].Capabilities[0] = taxonomy.Capability("tampered")
	got[0].Regions[0] = "tampered"

	if diff := cmp.Diff(offeringsSortedByName(), cat.Offerings()); diff != "" {
		t.Errorf("Offerings() changed after mutating a previous result (-want +got):\n%s", diff)
	}
}

func TestOfferingsFor(t *testing.T) {
	t.Parallel()

	cat := newCatalog(t, "scaleway", validSnapshot(), validOfferings())

	tests := []struct {
		name string
		in   taxonomy.Category
		want []string
	}{
		{name: "served by two offerings", in: taxonomy.ComputeVM, want: []string{"Elastic Metal", "Instances"}},
		{name: "served by one offering", in: taxonomy.StorageBlock, want: []string{"Elastic Metal"}},
		{name: "kubernetes", in: taxonomy.KubernetesManaged, want: []string{"Kapsule"}},
		{name: "postgresql", in: taxonomy.DatabasePostgreSQL, want: []string{"Managed Database for PostgreSQL"}},
		{name: "no offering serves it", in: taxonomy.DatabaseMySQL, want: nil},
		{name: "unclassified is never offered", in: taxonomy.Unclassified, want: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(tc.want, offeringNames(cat.OfferingsFor(tc.in))); diff != "" {
				t.Errorf("OfferingsFor(%q) mismatch (-want +got):\n%s", tc.in, diff)
			}
		})
	}
}

func TestOfferingHas(t *testing.T) {
	t.Parallel()

	offering := catalog.Offering{
		Name:         "Kapsule",
		Categories:   []taxonomy.Category{taxonomy.KubernetesManaged},
		Capabilities: []taxonomy.Capability{taxonomy.Autoscaling, taxonomy.HighAvailability},
		Regions:      []string{"fr-par"},
	}

	tests := []struct {
		name string
		in   taxonomy.Capability
		want bool
	}{
		{name: "listed", in: taxonomy.Autoscaling, want: true},
		{name: "listed last", in: taxonomy.HighAvailability, want: true},
		{name: "not listed", in: taxonomy.PointInTimeRestore, want: false},
		{name: "empty", in: taxonomy.Capability(""), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := offering.Has(tc.in); got != tc.want {
				t.Errorf("Has(%q) = %t, want %t", tc.in, got, tc.want)
			}
		})
	}
}

func TestNewErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    string
		snapshot  catalog.Snapshot
		offerings []catalog.Offering
		wantErr   error
		wantMsg   []string
	}{
		{
			name:      "empty target",
			target:    "",
			snapshot:  validSnapshot(),
			offerings: validOfferings(),
			wantErr:   catalog.ErrInvalidCatalog,
		},
		{
			name:      "empty snapshot source url",
			target:    "scaleway",
			snapshot:  catalog.Snapshot{RetrievedOn: validSnapshot().RetrievedOn},
			offerings: validOfferings(),
			wantErr:   catalog.ErrInvalidCatalog,
		},
		{
			name:      "zero retrieval date",
			target:    "scaleway",
			snapshot:  catalog.Snapshot{SourceURL: validSnapshot().SourceURL},
			offerings: validOfferings(),
			wantErr:   catalog.ErrInvalidCatalog,
		},
		{
			name:      "empty offering name",
			target:    "scaleway",
			snapshot:  validSnapshot(),
			offerings: []catalog.Offering{{Categories: []taxonomy.Category{taxonomy.ComputeVM}}},
			wantErr:   catalog.ErrEmptyOfferingName,
		},
		{
			name:     "duplicate offering name",
			target:   "scaleway",
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{
				{Name: "Instances", Categories: []taxonomy.Category{taxonomy.ComputeVM}},
				{Name: "Instances", Categories: []taxonomy.Category{taxonomy.ComputeVM}},
			},
			wantErr: catalog.ErrDuplicateOffering,
			wantMsg: []string{"Instances"},
		},
		{
			name:      "offering serving no category",
			target:    "scaleway",
			snapshot:  validSnapshot(),
			offerings: []catalog.Offering{{Name: "Instances"}},
			wantErr:   catalog.ErrNoCategory,
			wantMsg:   []string{"Instances"},
		},
		{
			// A target never offers "unclassified": that is the absence of a category.
			name:     "offering serving unclassified",
			target:   "scaleway",
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{{
				Name:       "Mystery",
				Categories: []taxonomy.Category{taxonomy.Unclassified},
			}},
			wantErr: catalog.ErrInvalidCatalog,
			wantMsg: []string{"Mystery", "unclassified"},
		},
		{
			name:     "offering with unknown category",
			target:   "scaleway",
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{{
				Name:       "Instances",
				Categories: []taxonomy.Category{taxonomy.Category("compute.mv")},
			}},
			wantErr: taxonomy.ErrUnknownCategory,
			wantMsg: []string{"Instances", "compute.mv"},
		},
		{
			name:     "offering with unknown capability",
			target:   "scaleway",
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{{
				Name:         "Instances",
				Categories:   []taxonomy.Category{taxonomy.ComputeVM},
				Capabilities: []taxonomy.Capability{taxonomy.Capability("pitr")},
			}},
			wantErr: taxonomy.ErrUnknownCapability,
			wantMsg: []string{"Instances", "pitr"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := catalog.New(tc.target, tc.snapshot, tc.offerings)
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
