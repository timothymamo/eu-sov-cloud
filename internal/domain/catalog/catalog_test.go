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
			Capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability, taxonomy.Autoscaling},
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
		Capabilities: []taxonomy.Capability{taxonomy.Autoscaling, taxonomy.NodeHighAvailability},
		Regions:      []string{"fr-par"},
	}

	tests := []struct {
		name string
		in   taxonomy.Capability
		want bool
	}{
		{name: "listed", in: taxonomy.Autoscaling, want: true},
		{name: "listed last", in: taxonomy.NodeHighAvailability, want: true},
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

// A snapshot may list only zone-level high availability: the taxonomy holds the
// implication zone level implies node level, and New applies it to every
// offering, so Offering.Has answers correctly everywhere (mapping, tests, a
// future comparison report). Resources are never expanded, so this is the one
// place the implication is applied. Capabilities stay sorted and de-duplicated,
// so a node-level need appears exactly once.
func TestNewExpandsImpliedCapabilities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		listed  []taxonomy.Capability
		want    []taxonomy.Capability
		hasNode bool
		hasZone bool
	}{
		{
			// The shape milestone 2 and 3 snapshots will have: only the zone
			// level is written down.
			name:    "zone level gives node level too",
			listed:  []taxonomy.Capability{taxonomy.ZoneHighAvailability},
			want:    []taxonomy.Capability{taxonomy.NodeHighAvailability, taxonomy.ZoneHighAvailability},
			hasNode: true,
			hasZone: true,
		},
		{
			// The implication runs downwards only: a node-level offering does not
			// survive the loss of a zone, so it must not claim the zone level.
			name:    "node level does not give zone level",
			listed:  []taxonomy.Capability{taxonomy.NodeHighAvailability},
			want:    []taxonomy.Capability{taxonomy.NodeHighAvailability},
			hasNode: true,
			hasZone: false,
		},
		{
			// A snapshot may list both levels. That is not a duplicate capability:
			// the catalog still holds the node level once.
			name:    "both levels listed stay valid",
			listed:  []taxonomy.Capability{taxonomy.ZoneHighAvailability, taxonomy.NodeHighAvailability},
			want:    []taxonomy.Capability{taxonomy.NodeHighAvailability, taxonomy.ZoneHighAvailability},
			hasNode: true,
			hasZone: true,
		},
		{
			// The implication reaches only the capability the taxonomy states it
			// for: everything else the snapshot lists is kept as given.
			name:    "an unrelated capability is left alone",
			listed:  []taxonomy.Capability{taxonomy.IPv6, taxonomy.ZoneHighAvailability},
			want:    []taxonomy.Capability{taxonomy.IPv6, taxonomy.NodeHighAvailability, taxonomy.ZoneHighAvailability},
			hasNode: true,
			hasZone: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cat := newCatalog(t, targetName, validSnapshot(), []catalog.Offering{{
				Name:         "Managed Database for PostgreSQL",
				Categories:   []taxonomy.Category{taxonomy.DatabasePostgreSQL},
				Capabilities: tc.listed,
				Regions:      []string{"fr-par"},
			}})

			offering := cat.Offerings()[0]

			if diff := cmp.Diff(tc.want, offering.Capabilities); diff != "" {
				t.Errorf("Capabilities mismatch (-want +got):\n%s", diff)
			}

			if got := offering.Has(taxonomy.NodeHighAvailability); got != tc.hasNode {
				t.Errorf("Has(node-high-availability) = %t, want %t", got, tc.hasNode)
			}

			if got := offering.Has(taxonomy.ZoneHighAvailability); got != tc.hasZone {
				t.Errorf("Has(zone-high-availability) = %t, want %t", got, tc.hasZone)
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
		// secret is the part of a rejected URL that must never be echoed. Saying
		// which character of a password is wrong would put that character in a
		// report, so the error must be silent about it.
		secret string
	}{
		// Accepted: a plain page, the same page with a query and a fragment, and
		// parentheses and escaped characters, which are legal URL characters.
		// Escaping them for a Markdown table is the reporter's job.
		{name: "pricing page", url: "https://www.scaleway.com/en/pricing/"},
		{name: "query and fragment", url: "https://www.scaleway.com/en/pricing/?tab=compute#instances"},
		{name: "parentheses", url: "https://example.com/a(b)"},
		{name: "escaped space", url: "https://example.com/a%20b"},
		{name: "sub domain", url: "https://sub-domain.example.com/x"},
		// Hardening against an HTML or Markdown renderer must not refuse what is
		// a plain URL: an ampersand between query parameters is not an entity,
		// and a percent escape that decodes to text is part of the URL.
		{name: "bare ampersands between query parameters", url: "https://example.com/?a=1&b=2"},
		{name: "ampersand and equals before a semicolon", url: "https://example.com/?a=1&b=2;c=3"},
		// An ampersand that starts no reference shape is a query parameter: a
		// named reference without its semicolon (&amp, &copy=2) is accepted on
		// purpose, because refusing it by prefix would refuse ordinary parameters
		// such as &region= and &section=.
		{name: "ampersand before a parameter called region", url: "https://example.com/?a=1&region=eu"},
		// An at sign is user info only in the authority, before the path or the
		// query starts.
		{name: "at sign in the path", url: "https://example.com/a@b"},
		{name: "at sign in the query", url: "https://example.com/?q=a@b"},
		{name: "percent encoded non ascii", url: "https://example.com/caf%C3%A9"},
		{name: "percent encoded ampersand", url: "https://example.com/?q=AT%26T"},
		{name: "percent encoded percent sign", url: "https://example.com/100%25"},

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
		{name: "userinfo with password", url: "https://user:pass@example.com/", wantErrs: []error{catalog.ErrInvalidCatalog}, secret: "pass"},
		{name: "look-alike host before at sign", url: "https://www.scaleway.com@evil.example/", wantErrs: []error{catalog.ErrInvalidCatalog}, secret: "www.scaleway.com"},
		{name: "port without host", url: "https://:443/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "opaque url", url: "https:example.com/opaque", wantErrs: []error{catalog.ErrInvalidCatalog}},

		// A host must be a plain DNS name. Percent-encoding lets an invisible,
		// blank or non-ASCII character into the host, which a reader cannot see and
		// a report would print as an ordinary-looking domain.
		{name: "percent encoded right to left override host", url: "https://%E2%80%AEmoc.elgoog/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "percent encoded hangul filler host", url: "https://%E3%85%A4/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "percent encoded braille blank host", url: "https://%E2%A0%80/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "percent encoded invalid byte host", url: "https://%ff/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "percent encoded no break space host", url: "https://%C2%A0/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		// A host that is only an escape for '%' is not a host name at all.
		{name: "percent encoded percent host", url: "https://%25/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		// Characters that are only legal in a path or a query, or not at all.
		{name: "parentheses in host", url: "https://exa(mple).com/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "ampersand in host", url: "https://example.com&evil/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		// An IP literal is not a host name, and square brackets anywhere in a URL
		// are the syntax of a link in Markdown.
		{name: "ipv6 literal host", url: "https://[2001:db8::1]/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "ipv4 literal host", url: "https://192.0.2.1/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		// A browser resolves a host whose last label is a number as an IPv4
		// address: one label of decimal digits, or of 0x and hexadecimal digits.
		// 0x7f.1 and 2130706433 are both 127.0.0.1.
		{name: "short hexadecimal ipv4 host", url: "https://0x7f.1/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "decimal integer ipv4 host", url: "https://2130706433/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "decimal last label", url: "https://example.123/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "hexadecimal last label with a lower x", url: "https://example.0x1f/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "hexadecimal last label with an upper X", url: "https://example.0X1F/", wantErrs: []error{catalog.ErrInvalidCatalog}},
		// A number anywhere but the last label is a name a browser keeps: the
		// first label may start with one, and a letter later makes the label text.
		{name: "host name starting with a digit", url: "https://1password.com/"},
		{name: "sub domain starting with a digit", url: "https://123.example.com/"},
		{name: "last label with letters and a digit", url: "https://example.co1/"},
		{name: "www sub domain", url: "https://www.example.com/"},
		{
			name:     "markdown image in path",
			url:      "https://example.com/a)![i](https://evil.example/x.png)",
			wantErrs: []error{catalog.ErrInvalidCatalog},
		},
		// A percent sign must start a complete two-digit escape, in a query too.
		{name: "invalid escape in query", url: "https://example.com/?q=%zz", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "truncated escape in path", url: "https://example.com/a%2", wantErrs: []error{catalog.ErrInvalidCatalog}},

		// Characters a URL does not allow as given.
		{name: "space", url: "https://example.com/a b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "double quote", url: "https://example.com/a\"b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "angle brackets", url: "https://example.com/<x>", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "backtick", url: "https://example.com/a`b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "pipe", url: "https://example.com/a|b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "backslash", url: "https://example.com/a\\b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "caret", url: "https://example.com/a^b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "braces", url: "https://example.com/a{b}", wantErrs: []error{catalog.ErrInvalidCatalog}},

		// Defence in depth: a report shows the source URL as a code span, and an
		// HTML or Markdown renderer would turn an entity reference into the
		// character it names, so the URL a reader sees would not be the URL the
		// catalog holds. Entity shapes are refused rather than rendered.
		{name: "named entity reference", url: "https://example.com/?q=&amp;", wantErrs: []error{catalog.ErrInvalidCatalog}},
		// A named entity followed by a digit is the shape some parsers accept and
		// others do not (&lt without its semicolon), so it is refused: the URL a
		// reader sees would depend on the renderer.
		{name: "named entity with digits", url: "https://example.com/a&lt1;b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "decimal numeric entity", url: "https://example.com/?q=&#38;", wantErrs: []error{catalog.ErrInvalidCatalog}},
		// HTML decodes a numeric reference without its semicolon too, so a bare
		// &#38 or &#x26 is a reference shape as well.
		{name: "decimal numeric reference without a semicolon", url: "https://example.com/?x&#38", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "decimal numeric reference without a semicolon before a letter", url: "https://example.com/?x&#38x", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "hex numeric entity with a lower x", url: "https://example.com/?q=&#x26;", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "hex numeric entity with an upper X", url: "https://example.com/?q=&#X26;", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "hex numeric reference with a lower x and no semicolon", url: "https://example.com/?x&#x26", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "hex numeric reference with an upper X and no semicolon", url: "https://example.com/?x&#X26", wantErrs: []error{catalog.ErrInvalidCatalog}},
		// An ampersand and a hash that start no numeric reference are plain text:
		// &# then a letter that is not x, or &#x then a letter that is not hex.
		{name: "ampersand and hash before a letter", url: "https://example.com/?a=1&#frag"},
		{name: "ampersand and hash and x before a non hex letter", url: "https://example.com/?a=1&#xyz"},

		// A percent escape decodes to a byte, and a byte that is not valid text
		// has nothing a report can show: %0A is a line break, %00 a NUL,
		// %E2%80%AE a bidi override and %FF no UTF-8 at all.
		{name: "escape decoding to a line feed", url: "https://example.com/a%0Ab", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "escape decoding to NUL", url: "https://example.com/a%00b", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "escape decoding to a right to left override", url: "https://example.com/a%E2%80%AEb", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "escape decoding to a right to left override in lower case hex", url: "https://example.com/a%e2%80%aeb", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "escape decoding to a byte that is not UTF-8", url: "https://example.com/a%FFb", wantErrs: []error{catalog.ErrInvalidCatalog}},
		{name: "right to left override escape in the query", url: "https://example.com/?q=%E2%80%AE", wantErrs: []error{catalog.ErrInvalidCatalog}},
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

			// The URL may carry credentials, so it is never echoed back.
			if strings.Contains(msg, tc.url) {
				t.Errorf("New() error = %q, want it not to echo the source URL", msg)
			}

			assertNoSecret(t, msg, tc.secret)
		})
	}
}

// assertNoSecret fails when msg echoes secret or any three-rune run of it: an
// error that prints part of a credential is as bad as one that prints it all.
func assertNoSecret(t *testing.T, msg, secret string) {
	t.Helper()

	if secret == "" {
		return
	}

	runes := []rune(secret)
	for i := 0; i+2 < len(runes); i++ {
		if part := string(runes[i : i+3]); strings.Contains(msg, part) {
			t.Errorf("error = %q, want it not to echo %q of the credential", msg, part)
		}
	}
}

// The reason a source URL is rejected must not depend on the credentials it
// carries: an error that named the offending character, or where it was, would
// put part of a password into a report. So a URL that differs from a rejected one
// only in its user info produces exactly the same message.
func TestSnapshotSourceURLErrorDoesNotEchoUserInfo(t *testing.T) {
	t.Parallel()

	baseline := sourceURLError(t, "https://user:pass@example.com/").Error()

	tests := []struct {
		name string
		url  string
		// secrets are the strings whose presence in the message would mean it
		// echoed the credentials: the secret itself, and the code point the
		// offending character has.
		secrets []string
	}{
		{
			name:    "non ascii password",
			url:     "https://user:péss@example.com/",
			secrets: []string{"péss", "U+00E9"},
		},
		{
			name:    "character an rfc 3986 url does not allow",
			url:     "https://user:pa^ss@example.com/",
			secrets: []string{"pa^ss", "pa", "U+005E"},
		},
		// A browser's URL parser reads the user info after a single slash, or with
		// none at all, as user info too, so the reader must find it there before it
		// names a character of the password.
		{
			name:    "single slash user info with a character an rfc 3986 url does not allow",
			url:     "https:/user:p^ss@example.com/",
			secrets: []string{"p^ss", "U+005E"},
		},
		{
			name:    "single slash user info with a non ascii password",
			url:     "https:/user:pässword@example.com/",
			secrets: []string{"pässword", "U+00E4"},
		},
		{
			name:    "no slash user info with a character an rfc 3986 url does not allow",
			url:     "https:user:p^ss@example.com/",
			secrets: []string{"p^ss", "U+005E"},
		},
		// A browser strips leading spaces and C0 controls before it parses a URL,
		// so what follows them is user info the reader must find before it names a
		// character of the password.
		{
			name:    "leading space and a zero width space in the password",
			url:     " https://user:p\u200bss@example.com/",
			secrets: []string{"U+200B"},
		},
		{
			name:    "leading space and a c0 control in the password",
			url:     " https://user:p\x01ss@example.com/",
			secrets: []string{"U+0001"},
		},
		{
			name:    "leading space and a soft hyphen in the password",
			url:     " https://user:p\u00adss@example.com/",
			secrets: []string{"U+00AD"},
		},
		// Some parsers do not treat a backslash as a separator, so this is user
		// info with a backslash password rather than a path.
		{
			name:    "backslash in the password",
			url:     "https://user:p\\ss@example.com/",
			secrets: []string{"U+005C"},
		},
		{
			name:    "zero width space then a backslash in the password",
			url:     "https://user:p\u200b\\ss@example.com/",
			secrets: []string{"U+200B"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := sourceURLError(t, tc.url).Error()

			if msg != baseline {
				t.Errorf("New() error = %q, want the same message as for a rejected URL with other user info (%q)", msg, baseline)
			}

			for _, secret := range tc.secrets {
				if strings.Contains(msg, secret) {
					t.Errorf("New() error = %q, want it not to echo %q", msg, secret)
				}
			}

			if strings.Contains(msg, tc.url) {
				t.Errorf("New() error = %q, want it not to echo the source URL", msg)
			}
		})
	}
}

// An '@' that is outside the authority is not user info, but the reader cannot
// know that before url.Parse has spoken: the check that runs before the parse
// gives the bare message for any URL holding an '@', and the checks that run
// after it keep their detail. ADR 0009 records the trade-off; this pins both
// sides of it, so a reordering of the checks cannot silently flip either.
func TestSnapshotSourceURLErrorDetailAroundUserInfoGuard(t *testing.T) {
	t.Parallel()

	baseline := sourceURLError(t, "https://user:pass@example.com/").Error()

	tests := []struct {
		name string
		url  string
		// bare is true when the message must be exactly the one for a URL
		// with user info: the failing check runs before url.Parse.
		bare bool
		// detail is what the message must keep when bare is false: the check
		// runs after url.Parse confirmed there is no user info.
		detail []string
	}{
		{
			name: "at sign in the path and an incomplete escape",
			url:  "https://example.com/a@b/%ZZ",
			bare: true,
		},
		{
			name:   "at sign in the path and an escape that is not UTF-8",
			url:    "https://example.com/a@b/%FF",
			detail: []string{"byte 24"},
		},
		{
			name:   "at sign in the query and an escape that decodes to a line feed",
			url:    "https://example.com/?q=a@b%0A",
			detail: []string{"U+000A"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := sourceURLError(t, tc.url).Error()

			if tc.bare {
				if msg != baseline {
					t.Errorf("New() error = %q, want the same message as for a rejected URL with other user info (%q)", msg, baseline)
				}
			} else if msg == baseline {
				t.Errorf("New() error = %q, want it to keep the detail of the check, not the bare message for user info", msg)
			}

			for _, part := range tc.detail {
				if !strings.Contains(msg, part) {
					t.Errorf("New() error = %q, want it to contain %q", msg, part)
				}
			}

			if strings.Contains(msg, tc.url) {
				t.Errorf("New() error = %q, want it not to echo the source URL", msg)
			}
		})
	}
}

// sourceURLError returns the error New rejects rawURL with, failing the test when
// the URL is accepted or the error does not say the catalog is invalid.
func sourceURLError(t *testing.T, rawURL string) error {
	t.Helper()

	snapshot := catalog.Snapshot{SourceURL: rawURL, RetrievedOn: validSnapshot().RetrievedOn}

	_, err := catalog.New(targetName, snapshot, validOfferings())
	if err == nil {
		t.Fatalf("New() with source URL %q returned no error, want one wrapping %v", rawURL, catalog.ErrInvalidCatalog)
	}

	if !errors.Is(err, catalog.ErrInvalidCatalog) {
		t.Fatalf("New() with source URL %q error = %v, want an error wrapping %v", rawURL, err, catalog.ErrInvalidCatalog)
	}

	return err
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
			// A run of spaces is not a difference: two names that collapse to the
			// same text would appear as the same offering in a report.
			name:     "duplicate offering name differing in inner whitespace",
			target:   targetName,
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{
				{Name: "Object Storage", Categories: []taxonomy.Category{taxonomy.StorageObject}, Regions: []string{"fr-par"}},
				{Name: "Object  Storage", Categories: []taxonomy.Category{taxonomy.StorageObject}, Regions: []string{"fr-par"}},
			},
			wantErr: catalog.ErrDuplicateOffering,
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
		{
			// ADR 0011 retired the unscoped value and catalog.New reads every
			// capability through taxonomy.ParseCapability, so a snapshot still
			// listing it is refused rather than silently read as one of the two
			// scopes. The error names the offering and quotes the retired value and
			// both replacements, which a file written before the split can act on.
			name:     "offering with retired high-availability capability",
			target:   targetName,
			snapshot: validSnapshot(),
			offerings: []catalog.Offering{{
				Name:         "Instances",
				Categories:   []taxonomy.Category{taxonomy.ComputeVM},
				Capabilities: []taxonomy.Capability{taxonomy.Capability("high-availability")},
				Regions:      []string{"fr-par"},
			}},
			wantErr: taxonomy.ErrUnknownCapability,
			// Each value is quoted: the retired value is a substring of
			// "node-high-availability", so the quotes keep the assertions apart.
			wantMsg: []string{
				text.Quote("Instances"),
				text.Quote("high-availability"),
				text.Quote("node-high-availability"),
				text.Quote("zone-high-availability"),
			},
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
		// Leading and trailing whitespace is padding, not part of the value: a
		// padded name or region is not the same string as the name it pads.
		{name: "leading space", value: " Scaleway"},
		{name: "trailing space", value: "Scaleway "},
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

// A non-ASCII space separator is legitimate text inside a name or a region: a
// target may sell "Offre\u00a0Pro" and name a region in its own language. Only an
// ID, which a reader retypes, rejects these characters, so an offering keeps them
// exactly as given.
func TestNewAcceptsNonASCIISpaceInFreeText(t *testing.T) {
	t.Parallel()

	separators := []struct {
		name string
		sep  string
	}{
		{name: "no break space", sep: "\u00a0"},
		{name: "ogham space mark", sep: "\u1680"},
		{name: "figure space", sep: "\u2007"},
		{name: "narrow no break space", sep: "\u202f"},
		{name: "ideographic space", sep: "\u3000"},
	}

	for _, tc := range separators {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			name := "Offre" + tc.sep + "Pro"
			region := "fr" + tc.sep + "par"

			cat := newCatalog(t, targetName, validSnapshot(), []catalog.Offering{{
				Name:       name,
				Categories: []taxonomy.Category{taxonomy.ComputeVM},
				Regions:    []string{region},
			}})

			got := cat.Offerings()
			if len(got) != 1 {
				t.Fatalf("Offerings() returned %d offerings, want 1", len(got))
			}

			if got[0].Name != name {
				t.Errorf("Name = %q, want %q stored as given", got[0].Name, name)
			}

			if diff := cmp.Diff([]string{region}, got[0].Regions); diff != "" {
				t.Errorf("Regions are not stored as given (-want +got):\n%s", diff)
			}
		})
	}
}

// Duplicate detection ignores runs of inner whitespace, but a stored value is
// never normalised: a report shows the target's own spelling of the name.
func TestNewKeepsNameAsGiven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		offeringName string
	}{
		{name: "single inner space", offeringName: "Object Storage"},
		{name: "double inner space", offeringName: "Object  Storage"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cat := newCatalog(t, targetName, validSnapshot(), []catalog.Offering{{
				Name:       tc.offeringName,
				Categories: []taxonomy.Category{taxonomy.StorageObject},
				Regions:    []string{"fr-par"},
			}})

			got := cat.Offerings()
			if len(got) != 1 {
				t.Fatalf("Offerings() returned %d offerings, want 1", len(got))
			}

			if got[0].Name != tc.offeringName {
				t.Errorf("Name = %q, want %q stored as given", got[0].Name, tc.offeringName)
			}
		})
	}
}
