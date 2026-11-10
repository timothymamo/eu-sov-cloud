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
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// The target is the human-readable display name a report shows, not a slug:
// Target() returns exactly what was passed, with no case mapping.
const targetName = "Scaleway"

// validSnapshot is the dated, sourced snapshot every real catalog must carry.
func validSnapshot() catalog.Snapshot {
	return catalog.Snapshot{
		SourceURL:   "https://www.scaleway.com/en/pricing/",
		RetrievedOn: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}
}

// validOfferings is deliberately not in name order, and its inner slices are
// deliberately unsorted and duplicated: New must sort and de-duplicate them.
// Elastic Metal, Instances, Kapsule, Managed Database for PostgreSQL, Managed
// Database for PostgreSQL and MySQL, Object Storage is the expected order.
func validOfferings() []catalog.Offering {
	return []catalog.Offering{
		{
			Name:         "Object Storage",
			Categories:   []taxonomy.Category{taxonomy.StorageObject, taxonomy.StorageObject},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6, taxonomy.ObjectVersioning, taxonomy.IPv6},
			Regions:      []string{"pl-waw", "fr-par", "fr-par"},
		},
		{
			Name:         "Instances",
			Categories:   []taxonomy.Category{taxonomy.ComputeVM},
			Capabilities: []taxonomy.Capability{taxonomy.Autoscaling, taxonomy.IPv6, taxonomy.Autoscaling},
			Regions:      []string{"nl-ams", "fr-par", "nl-ams", "pl-waw"},
		},
		{
			// Elastic Metal is a dedicated server: compute.vm only, no block storage.
			Name:         "Elastic Metal",
			Categories:   []taxonomy.Category{taxonomy.ComputeVM},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
			Regions:      []string{"fr-par", "nl-ams"},
		},
		{
			Name:         "Kapsule",
			Categories:   []taxonomy.Category{taxonomy.KubernetesManaged},
			Capabilities: []taxonomy.Capability{taxonomy.HighAvailability, taxonomy.Autoscaling},
			Regions:      []string{"pl-waw", "nl-ams", "fr-par"},
		},
		{
			Name:         "Managed Database for PostgreSQL",
			Categories:   []taxonomy.Category{taxonomy.DatabasePostgreSQL},
			Capabilities: []taxonomy.Capability{taxonomy.ReadReplicas, taxonomy.PointInTimeRestore},
			Regions:      []string{"fr-par", "nl-ams", "pl-waw"},
		},
		{
			// One offering may serve several categories.
			Name:         "Managed Database for PostgreSQL and MySQL",
			Categories:   []taxonomy.Category{taxonomy.DatabaseMySQL, taxonomy.DatabasePostgreSQL},
			Capabilities: []taxonomy.Capability{taxonomy.PointInTimeRestore, taxonomy.ReadReplicas, taxonomy.ReadReplicas},
			Regions:      []string{"nl-ams", "fr-par", "pl-waw", "fr-par"},
		},
	}
}

// sortedUnique returns a copy of in sorted ascending and de-duplicated.
func sortedUnique[T ~string](in []T) []T {
	out := slices.Clone(in)
	slices.Sort(out)

	return slices.Compact(out)
}

// offeringsSortedByName is what the catalog must hold: name order, and every
// inner slice sorted ascending and de-duplicated.
func offeringsSortedByName() []catalog.Offering {
	offerings := validOfferings()
	for i, o := range offerings {
		o.Categories = sortedUnique(o.Categories)
		o.Capabilities = sortedUnique(o.Capabilities)
		o.Regions = sortedUnique(o.Regions)
		offerings[i] = o
	}

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

	cat := newCatalog(t, targetName, validSnapshot(), validOfferings())

	// No case mapping: a slug such as "scaleway" would come back unchanged.
	if got := cat.Target(); got != targetName {
		t.Errorf("Target() = %q, want %q", got, targetName)
	}

	if diff := cmp.Diff(validSnapshot(), cat.Snapshot()); diff != "" {
		t.Errorf("Snapshot() mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(offeringsSortedByName(), cat.Offerings()); diff != "" {
		t.Errorf("Offerings() are not sorted by name with sorted inner slices (-want +got):\n%s", diff)
	}
}

func TestOfferingsReturnsCopy(t *testing.T) {
	t.Parallel()

	cat := newCatalog(t, targetName, validSnapshot(), validOfferings())

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

func TestOfferingsForReturnsCopy(t *testing.T) {
	t.Parallel()

	cat := newCatalog(t, targetName, validSnapshot(), validOfferings())

	got := cat.OfferingsFor(taxonomy.DatabasePostgreSQL)
	if len(got) == 0 {
		t.Fatal("OfferingsFor(database.postgresql) is empty")
	}

	got[0].Name = "Tampered"
	got[0].Categories[0] = taxonomy.Category("tampered")
	got[0].Capabilities[0] = taxonomy.Capability("tampered")
	got[0].Regions[0] = "tampered"

	want := []string{"Managed Database for PostgreSQL", "Managed Database for PostgreSQL and MySQL"}
	if diff := cmp.Diff(want, offeringNames(cat.OfferingsFor(taxonomy.DatabasePostgreSQL))); diff != "" {
		t.Errorf("OfferingsFor() changed after mutating a previous result (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(offeringsSortedByName(), cat.Offerings()); diff != "" {
		t.Errorf("Offerings() changed after mutating an OfferingsFor() result (-want +got):\n%s", diff)
	}
}

// New takes ownership: mutating the slice handed to it, or the slices inside it,
// must not reach the catalog.
func TestNewDoesNotAliasInput(t *testing.T) {
	t.Parallel()

	offerings := validOfferings()
	cat := newCatalog(t, targetName, validSnapshot(), offerings)

	offerings[0].Name = "Tampered"
	offerings[0].Categories[0] = taxonomy.Category("tampered")
	offerings[0].Capabilities[0] = taxonomy.Capability("tampered")
	offerings[0].Regions[0] = "tampered"

	if diff := cmp.Diff(offeringsSortedByName(), cat.Offerings()); diff != "" {
		t.Errorf("Offerings() changed after mutating the input slice (-want +got):\n%s", diff)
	}
}

func TestOfferingsFor(t *testing.T) {
	t.Parallel()

	cat := newCatalog(t, targetName, validSnapshot(), validOfferings())

	tests := []struct {
		name string
		in   taxonomy.Category
		want []string
	}{
		{name: "compute served by two offerings", in: taxonomy.ComputeVM, want: []string{"Elastic Metal", "Instances"}},
		{
			// One offering serving several categories: PostgreSQL and MySQL.
			name: "postgresql served by two offerings",
			in:   taxonomy.DatabasePostgreSQL,
			want: []string{"Managed Database for PostgreSQL", "Managed Database for PostgreSQL and MySQL"},
		},
		{name: "mysql", in: taxonomy.DatabaseMySQL, want: []string{"Managed Database for PostgreSQL and MySQL"}},
		{name: "kubernetes", in: taxonomy.KubernetesManaged, want: []string{"Kapsule"}},
		{name: "object storage", in: taxonomy.StorageObject, want: []string{"Object Storage"}},
		{name: "no offering serves block storage", in: taxonomy.StorageBlock, want: nil},
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

// The retrieval instant is normalised to UTC: report renderers format it as a
// UTC calendar date, so keeping the caller's zone would shift the date.
func TestSnapshotRetrievedOnIsUTC(t *testing.T) {
	t.Parallel()

	snapshot := catalog.Snapshot{
		SourceURL:   "https://www.scaleway.com/en/pricing/",
		RetrievedOn: time.Date(2026, 11, 1, 20, 0, 0, 0, time.FixedZone("x", -10*60*60)),
	}

	cat := newCatalog(t, targetName, snapshot, validOfferings())

	got := cat.Snapshot()
	if got.RetrievedOn.Location() != time.UTC {
		t.Errorf("Snapshot().RetrievedOn.Location() = %v, want UTC", got.RetrievedOn.Location())
	}

	if !got.RetrievedOn.Equal(snapshot.RetrievedOn) {
		t.Errorf("Snapshot().RetrievedOn = %v, want the same instant as %v", got.RetrievedOn, snapshot.RetrievedOn)
	}

	if got.SourceURL != snapshot.SourceURL {
		t.Errorf("Snapshot().SourceURL = %q, want %q", got.SourceURL, snapshot.SourceURL)
	}
}

// SourceURL is what a report cites as the origin of the data, so it must be a
// plain absolute https URL: no credentials, no host hidden behind an at sign, no
// characters that are unsafe outside a URL. It is free text as well, so control
// characters and over-long values are rejected by text.Validate. Errors name the
// source URL and stay bounded whatever the input.
func TestSnapshotSourceURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		// wantErrs are the sentinel errors the error must wrap; nil means the URL
		// is accepted.
		wantErrs []error
	}{
		// Accepted: a plain page, the same page with a query and a fragment, and
		// parentheses, which are legal URL characters. Escaping them for a Markdown
		// table is the reporter's job.
		{name: "pricing page", url: "https://www.scaleway.com/en/pricing/"},
		{name: "query and fragment", url: "https://www.scaleway.com/en/pricing/?tab=compute#instances"},
		{name: "parentheses", url: "https://example.com/a(b)"},

		// Invalid free text: rejected before the URL is even parsed.
		{
			name:     "right to left override",
			url:      "https://example.com/a\u202eb",
			wantErrs: []error{catalog.ErrInvalidCatalog, text.ErrInvalid},
		},
		{
			name:     "longer than max len",
			url:      "https://example.com/" + strings.Repeat("a", text.MaxLen),
			wantErrs: []error{catalog.ErrInvalidCatalog, text.ErrInvalid},
		},

		// Not a plain page: credentials in the URL would end up in a report, and a
		// host in front of an at sign hides the real one.
		{name: "userinfo with password", url: "https://user:pass@example.com/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "look-alike host before at sign", url: "https://www.scaleway.com@evil.example/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "port without host", url: "https://:443/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "opaque url", url: "https:example.com/opaque", wantErrs: []error{catalog.ErrInvalidCatalog}},

		// Characters a URL does not allow as given.
		{name: "space", url: "https://example.com/a b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "double quote", url: "https://example.com/a\"b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "angle brackets", url: "https://example.com/<x>", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "backtick", url: "https://example.com/a`b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "pipe", url: "https://example.com/a|b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "backslash", url: "https://example.com/a\\b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "caret", url: "https://example.com/a^b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "braces", url: "https://example.com/a{b}", wantErrs: []error{catalog.ErrInvalidCatalog}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			snapshot := catalog.Snapshot{SourceURL: tc.url, RetrievedOn: validSnapshot().RetrievedOn}

			_, err := catalog.New(targetName, snapshot, validOfferings())
			if len(tc.wantErrs) == 0 {
				if err != nil {
					t.Fatalf("New() with source URL %q returned unexpected error: %v", tc.url, err)
				}

				return
			}

			for _, wantErr := range tc.wantErrs {
				if !errors.Is(err, wantErr) {
					t.Fatalf("New() with source URL %q error = %v, want an error wrapping %v", tc.url, err, wantErr)
				}
			}

			msg := err.Error()
			if lower := strings.ToLower(msg); !strings.Contains(lower, "source url") {
				t.Errorf("New() error = %q, want it to name the field %q", msg, "source URL")
			}
		})
	}
}

// A resource with no capabilities and one with an empty capability list are the
// same offering; callers must not have to care which spelling a fixture uses.
func TestNewCanonicalisesEmptyCapabilities(t *testing.T) {
	t.Parallel()

	withNil := catalog.Offering{
		Name:         "Instances",
		Categories:   []taxonomy.Category{taxonomy.ComputeVM},
		Capabilities: nil,
		Regions:      []string{"fr-par"},
	}

	withEmpty := catalog.Offering{
		Name:         "Instances",
		Categories:   []taxonomy.Category{taxonomy.ComputeVM},
		Capabilities: []taxonomy.Capability{},
		Regions:      []string{"fr-par"},
	}

	nilCat := newCatalog(t, targetName, validSnapshot(), []catalog.Offering{withNil})
	emptyCat := newCatalog(t, targetName, validSnapshot(), []catalog.Offering{withEmpty})

	if got := nilCat.Offerings()[0].Capabilities; got != nil {
		t.Errorf("Capabilities = %#v for nil input, want nil", got)
	}

	if got := emptyCat.Offerings()[0].Capabilities; got != nil {
		t.Errorf("Capabilities = %#v for empty input, want nil", got)
	}

	if diff := cmp.Diff(nilCat.Offerings(), emptyCat.Offerings()); diff != "" {
		t.Errorf("nil and empty capabilities are not canonicalised to the same offering (-nil +empty):\n%s", diff)
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
			name:      "whitespace only target",
			target:    "   ",
			snapshot:  validSnapshot(),
			offerings: validOfferings(),
			wantErr:   catalog.ErrInvalidCatalog,
		},
		{
			name:      "empty snapshot source url",
			target:    targetName,
			snapshot:  catalog.Snapshot{RetrievedOn: validSnapshot().RetrievedOn},
			offerings: validOfferings(),
			wantErr:   catalog.ErrInvalidCatalog,
		},
		{
			name:      "plain http source url",
			target:    targetName,
			snapshot:  catalog.Snapshot{SourceURL: "http://www.scaleway.com/en/pricing/", RetrievedOn: validSnapshot().RetrievedOn},
			offerings: validOfferings(),
			wantErr:   catalog.ErrInvalidCatalog,
		},
		{
			name:      "javascript source url",
			target:    targetName,
			snapshot:  catalog.Snapshot{SourceURL: "javascript:alert(1)", RetrievedOn: validSnapshot().RetrievedOn},
			offerings: validOfferings(),
			wantErr:   catalog.ErrInvalidCatalog,
		},
		{
			name:      "relative source url",
			target:    targetName,
			snapshot:  catalog.Snapshot{SourceURL: "www.scaleway.com/en/pricing/", RetrievedOn: validSnapshot().RetrievedOn},
			offerings: validOfferings(),
			wantErr:   catalog.ErrInvalidCatalog,
		},
		{
			name:      "https source url without host",
			target:    targetName,
			snapshot:  catalog.Snapshot{SourceURL: "https://", RetrievedOn: validSnapshot().RetrievedOn},
			offerings: validOfferings(),
			wantErr:   catalog.ErrInvalidCatalog,
		},
		{
			name:      "zero retrieval date",
			target:    targetName,
			snapshot:  catalog.Snapshot{SourceURL: validSnapshot().SourceURL},
			offerings: validOfferings(),
			wantErr:   catalog.ErrInvalidCatalog,
		},
		{
			name:      "empty offering name",
			target:    targetName,
			snapshot:  validSnapshot(),
			offerings: []catalog.Offering{{Categories: []taxonomy.Category{taxonomy.ComputeVM}, Regions: []string{"fr-par"}}},
			wantErr:   catalog.ErrEmptyOfferingName,
		},
		{
			name:      "whitespace only offering name",
			target:    targetName,
			snapshot:  validSnapshot(),
			offerings: []catalog.Offering{{Name: "   ", Categories: []taxonomy.Category{taxonomy.ComputeVM}, Regions: []string{"fr-par"}}},
			wantErr:   catalog.ErrEmptyOfferingName,
		},
		{
			name:     "duplicate offering name",
			target:   targetName,
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{
				{Name: "Instances", Categories: []taxonomy.Category{taxonomy.ComputeVM}, Regions: []string{"fr-par"}},
				{Name: "Instances", Categories: []taxonomy.Category{taxonomy.ComputeVM}, Regions: []string{"fr-par"}},
			},
			wantErr: catalog.ErrDuplicateOffering,
			wantMsg: []string{"Instances"},
		},
		{
			name:      "offering serving no category",
			target:    targetName,
			snapshot:  validSnapshot(),
			offerings: []catalog.Offering{{Name: "Instances", Regions: []string{"fr-par"}}},
			wantErr:   catalog.ErrNoCategory,
			wantMsg:   []string{"Instances"},
		},
		{
			name:      "offering without regions",
			target:    targetName,
			snapshot:  validSnapshot(),
			offerings: []catalog.Offering{{Name: "Instances", Categories: []taxonomy.Category{taxonomy.ComputeVM}}},
			wantErr:   catalog.ErrNoRegion,
			wantMsg:   []string{"Instances"},
		},
		{
			name:     "offering with empty regions",
			target:   targetName,
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{{
				Name:       "Instances",
				Categories: []taxonomy.Category{taxonomy.ComputeVM},
				Regions:    []string{},
			}},
			wantErr: catalog.ErrNoRegion,
			wantMsg: []string{"Instances"},
		},
		{
			name:     "offering with a blank region among valid ones",
			target:   targetName,
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{{
				Name:       "Instances",
				Categories: []taxonomy.Category{taxonomy.ComputeVM},
				Regions:    []string{"fr-par", "   "},
			}},
			wantErr: catalog.ErrNoRegion,
			wantMsg: []string{"Instances"},
		},
		{
			name:     "offering with an empty region name",
			target:   targetName,
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{{
				Name:       "Instances",
				Categories: []taxonomy.Category{taxonomy.ComputeVM},
				Regions:    []string{""},
			}},
			wantErr: catalog.ErrNoRegion,
			wantMsg: []string{"Instances"},
		},
		{
			// A target never offers "unclassified": that is the absence of a category.
			name:     "offering serving unclassified",
			target:   targetName,
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{{
				Name:       "Mystery",
				Categories: []taxonomy.Category{taxonomy.Unclassified},
				Regions:    []string{"fr-par"},
			}},
			wantErr: catalog.ErrInvalidCatalog,
			wantMsg: []string{"Mystery", "unclassified"},
		},
		{
			name:     "offering with unknown category",
			target:   targetName,
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{{
				Name:       "Instances",
				Categories: []taxonomy.Category{taxonomy.Category("compute.mv")},
				Regions:    []string{"fr-par"},
			}},
			wantErr: taxonomy.ErrUnknownCategory,
			wantMsg: []string{"Instances", "compute.mv"},
		},
		{
			name:     "offering with unknown capability",
			target:   targetName,
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{{
				Name:         "Instances",
				Categories:   []taxonomy.Category{taxonomy.ComputeVM},
				Capabilities: []taxonomy.Capability{taxonomy.Capability("pitr")},
				Regions:      []string{"fr-par"},
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

// Free text from a catalog reaches reports and terminals, so control characters,
// bidi overrides, invalid UTF-8 and over-long values are rejected instead of
// rendered. The error names the field and, when it has one, the offering.
func TestNewRejectsInvalidText(t *testing.T) {
	t.Parallel()

	invalid := []struct {
		name  string
		value string
	}{
		{name: "ansi escape", value: "\x1b[2J"},
		{name: "newline", value: "row\nvalue"},
		{name: "right to left override", value: "a\u202eb"},
		{name: "zero width space", value: "a\u200bb"},
		// Some renderers treat Zl and Zp as line breaks, the same risk as a newline.
		{name: "line separator", value: "a\u2028b"},
		{name: "paragraph separator", value: "a\u2029b"},
		{name: "invalid utf8", value: "a\xffb"},
		{name: "longer than max len", value: strings.Repeat("a", text.MaxLen+1)},
	}

	type invalidCase struct {
		name         string
		target       string
		offering     catalog.Offering
		wantField    string
		wantOffering string
		// wantErrs are the sentinel errors the error must wrap. The target is part
		// of the catalog's own validity, so its errors say ErrInvalidCatalog too;
		// an offering's name or region is only invalid free text.
		wantErrs []error
	}

	tests := make([]invalidCase, 0, len(invalid)*3)

	for _, tc := range invalid {
		tests = append(tests, invalidCase{
			name:      "target/" + tc.name,
			target:    tc.value,
			offering:  catalog.Offering{Name: "Instances", Categories: []taxonomy.Category{taxonomy.ComputeVM}, Regions: []string{"fr-par"}},
			wantField: "target",
			wantErrs:  []error{catalog.ErrInvalidCatalog, text.ErrInvalid},
		})

		tests = append(tests, invalidCase{
			name:      "offering name/" + tc.name,
			target:    targetName,
			offering:  catalog.Offering{Name: tc.value, Categories: []taxonomy.Category{taxonomy.ComputeVM}, Regions: []string{"fr-par"}},
			wantField: "name",
			wantErrs:  []error{text.ErrInvalid},
		})

		tests = append(tests, invalidCase{
			name:         "region/" + tc.name,
			target:       targetName,
			offering:     catalog.Offering{Name: "Instances", Categories: []taxonomy.Category{taxonomy.ComputeVM}, Regions: []string{"fr-par", tc.value}},
			wantField:    "region",
			wantOffering: "Instances",
			wantErrs:     []error{text.ErrInvalid},
		})
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := catalog.New(tc.target, validSnapshot(), []catalog.Offering{tc.offering})
			for _, wantErr := range tc.wantErrs {
				if !errors.Is(err, wantErr) {
					t.Fatalf("New() error = %v, want an error wrapping %v", err, wantErr)
				}
			}

			if err == nil {
				t.Fatalf("New() returned no error, want an error naming the field %q", tc.wantField)
			}

			msg := err.Error()
			if lower := strings.ToLower(msg); !strings.Contains(lower, tc.wantField) {
				t.Errorf("New() error = %q, want it to name the field %q", msg, tc.wantField)
			}

			if tc.wantOffering != "" && !strings.Contains(msg, tc.wantOffering) {
				t.Errorf("New() error = %q, want it to name the offering %q", msg, tc.wantOffering)
			}
		})
	}
}
