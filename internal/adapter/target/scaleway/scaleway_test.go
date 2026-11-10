// Package scaleway_test pins the Scaleway catalog adapter: the target contract in
// internal/adapter/target/targettest, the snapshot header the catalog reports, a
// golden change detector for the parsed catalog, the declared EU regions, and the
// adapter's own error contract - one line, the sentinel it wraps, every value
// shown through text.Quote, the keys a document must carry, the declared regions
// it must declare once and validly, and the size limit above which a document is
// refused undecoded. It is black-box: only the package's exported API is used.
package scaleway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/scaleway"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/targettest"
	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
	"github.com/timothymamo/eu-sov-cloud/internal/testutil"
)

// The adapter implements the catalog source port: pin that at compile time.
var _ analysis.CatalogSource = scaleway.New()

// The catalog the adapter embeds, and the golden file its parsed form renders to.
// Both are relative to this package's directory.
const (
	catalogFile = "catalog.json"
	goldenFile  = "testdata/catalog.golden"
)

// wantRegions is the EU region list catalog.json declares, written out here so
// the test cannot agree with a wrong declaration by construction: the contract
// expectations below and TestRegions compare against this list, never against
// what Regions() returns.
func wantRegions() []string {
	return []string{"fr-par", "it-mil", "nl-ams", "pl-waw"}
}

// TestCatalogContract runs the target contract suite every target adapter must
// pass: the display name, every offering only in a declared EU region, the four
// categories PRD 2 requires, the same catalog on every call, and cancellation.
func TestCatalogContract(t *testing.T) {
	t.Parallel()

	targettest.Run(t, scaleway.New(), targettest.Expectations{
		Target:  "Scaleway",
		Regions: wantRegions(),
		Categories: []taxonomy.Category{
			taxonomy.ComputeVM,
			taxonomy.KubernetesManaged,
			taxonomy.StorageObject,
			taxonomy.DatabasePostgreSQL,
		},
	})
}

// catalogOf returns the catalog a fresh source serves.
func catalogOf(t *testing.T) catalog.Catalog {
	t.Helper()

	cat, err := scaleway.New().Catalog(t.Context())
	if err != nil {
		t.Fatalf("Catalog() returned unexpected error: %v", err)
	}

	return cat
}

// TestCatalogSnapshotHeader pins that the snapshot comes from the file header: the
// documentation root the data was compiled from, and its retrieval date as UTC
// midnight. A report states both, so neither may be invented by the adapter.
func TestCatalogSnapshotHeader(t *testing.T) {
	t.Parallel()

	snapshot := catalogOf(t).Snapshot()

	const wantSourceURL = "https://www.scaleway.com/en/docs/"
	if snapshot.SourceURL != wantSourceURL {
		t.Errorf("Snapshot().SourceURL = %s, want %s", text.Quote(snapshot.SourceURL), text.Quote(wantSourceURL))
	}

	wantRetrievedOn := time.Date(2026, time.November, 10, 0, 0, 0, 0, time.UTC)
	if !snapshot.RetrievedOn.Equal(wantRetrievedOn) {
		t.Errorf("Snapshot().RetrievedOn = %s, want %s",
			snapshot.RetrievedOn.Format(time.RFC3339), wantRetrievedOn.Format(time.RFC3339))
	}

	if got := snapshot.RetrievedOn.Location(); got != time.UTC {
		t.Errorf("Snapshot().RetrievedOn location = %v, want time.UTC", got)
	}
}

// TestCatalogGolden pins the whole parsed catalog against a hand-written golden
// file: every offering with its categories, capabilities and regions, in the
// order and canonical form the domain gives them. It is a change detector for the
// snapshot data and for the parsing, so a refresh of catalog.json has to update
// this file in the same review.
func TestCatalogGolden(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	testutil.Golden(t, goldenFile, dump(cat))

	// The embed and Parse must agree: the golden file is the specification of
	// both, and the analysis may reach the catalog either way.
	data, err := os.ReadFile(catalogFile)
	if err != nil {
		t.Fatalf("reading the embedded catalog %s: %v", text.Quote(catalogFile), err)
	}

	parsed, err := scaleway.Parse(data)
	if err != nil {
		t.Fatalf("Parse() of %s returned unexpected error: %v", text.Quote(catalogFile), err)
	}

	if diff := cmp.Diff(string(dump(cat)), string(dump(parsed))); diff != "" {
		t.Errorf("Parse() of the embedded file differs from Catalog() (-Catalog +Parse):\n%s", diff)
	}
}

// dump renders cat as the golden file's lines: a header with where and when the
// data was taken, then one line per offering, sorted by name, with every list
// comma-separated and an empty list shown as "-".
func dump(cat catalog.Catalog) []byte {
	var b strings.Builder

	snapshot := cat.Snapshot()

	fmt.Fprintf(&b, "target: %s\n", cat.Target())
	fmt.Fprintf(&b, "sourceURL: %s\n", snapshot.SourceURL)
	fmt.Fprintf(&b, "retrievedOn: %s\n", snapshot.RetrievedOn.UTC().Format(time.DateOnly))

	for _, o := range cat.Offerings() {
		fmt.Fprintf(&b, "%s | %s | %s | %s\n",
			o.Name, join(o.Categories), join(o.Capabilities), join(o.Regions))
	}

	return []byte(b.String())
}

// join formats values as a comma-separated list, or "-" when there are none.
func join[S ~string](values []S) string {
	if len(values) == 0 {
		return "-"
	}

	strs := make([]string, len(values))
	for i, v := range values {
		strs[i] = string(v)
	}

	return strings.Join(strs, ", ")
}

// TestRegions pins the declared EU region list the reviewer checks against
// Scaleway's published regions, and that callers get a copy: a driving adapter
// cannot change what the adapter declares.
func TestRegions(t *testing.T) {
	t.Parallel()

	want := wantRegions()

	got := scaleway.Regions()
	if !slices.Equal(got, want) {
		t.Fatalf("Regions() = %v, want %v", got, want)
	}

	for i := range got {
		got[i] = "tampered"
	}

	if again := scaleway.Regions(); !slices.Equal(again, want) {
		t.Errorf("Regions() after changing the returned slice = %v, want %v: the result is not a copy", again, want)
	}
}

// validDocument is a complete, minimal catalog document: one offering in one
// declared region. The error table replaces one line of it to break exactly one
// thing.
const validDocument = `{
  "schemaVersion": 1,
  "target": "Scaleway",
  "sourceURL": "https://www.scaleway.com/en/docs/",
  "retrievedOn": "2026-11-10",
  "regions": ["fr-par"],
  "offerings": [
    {
      "name": "Instances",
      "categories": ["compute.vm"],
      "capabilities": ["ipv6"],
      "regions": ["fr-par"]
    }
  ]
}`

// doc returns validDocument with each old, new pair applied in order, so a case
// shows the one thing it breaks and leaves every other field valid. A pair whose
// old text is absent fails the test rather than leaving the document valid by
// accident.
func doc(t *testing.T, replacements ...string) string {
	t.Helper()

	data := validDocument

	for i := 0; i+1 < len(replacements); i += 2 {
		old, replacement := replacements[i], replacements[i+1]

		if !strings.Contains(data, old) {
			t.Fatalf("doc(): %s is not in the document", text.Quote(old))
		}

		data = strings.Replace(data, old, replacement, 1)
	}

	return data
}

// padTo returns data padded with trailing spaces to exactly n bytes. White space
// after the catalog object is allowed, so the result is the same document.
func padTo(t *testing.T, data string, n int) []byte {
	t.Helper()

	if len(data) > n {
		t.Fatalf("padTo(): the document is %d bytes, longer than %d", len(data), n)
	}

	return append([]byte(data), bytes.Repeat([]byte(" "), n-len(data))...)
}

// without returns validDocument with the top-level key removed, so a case pins a
// key the format requires. Re-encoding a map of raw fields keeps every nested
// value as it is; only the order of the top-level keys changes, which no test
// here depends on.
func without(t *testing.T, key string) string {
	t.Helper()

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(validDocument), &fields); err != nil {
		t.Fatalf("unmarshalling validDocument: %v", err)
	}

	if _, ok := fields[key]; !ok {
		t.Fatalf("without(): validDocument has no %s key", text.Quote(key))
	}

	delete(fields, key)

	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshalling the document without %s: %v", text.Quote(key), err)
	}

	return string(data)
}

// TestParseErrors pins every way parsing a catalog document fails: the sentinel
// the error wraps, and that the message is one line naming the offending value
// through text.Quote. A document the adapter cannot understand is never silently
// dropped or defaulted: an offering in a region the file does not declare, or an
// unknown capability, must fail rather than reach a report.
func TestParseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		// wantErr lists every sentinel the error must wrap. An empty list pins
		// that the document parses.
		wantErr []error
		// wantNotErr lists sentinels the error must not wrap, so a case pins
		// which of two plausible sentinels is the right one.
		wantNotErr []error
		// wantQuoted lists values the message must show through text.Quote.
		wantQuoted []string
		// wantContains lists substrings the message must contain and wantAbsent
		// ones it must not, for wording no single text.Quote value carries.
		wantContains []string
		wantAbsent   []string
	}{
		{
			name: "valid document",
			data: validDocument,
		},
		{
			name:    "not JSON",
			data:    "not json",
			wantErr: []error{scaleway.ErrDecode},
		},
		{
			name: "unknown top-level field",
			data: doc(t, `  "schemaVersion": 1,`,
				"  \"schemaVersion\": 1,\n  \"bogusField\": true,"),
			wantErr:    []error{scaleway.ErrDecode},
			wantQuoted: []string{"bogusField"},
		},
		{
			name: "unknown offering field",
			data: doc(t, `      "name": "Instances",`,
				"      \"name\": \"Instances\",\n      \"bogusOfferingField\": \"x\","),
			wantErr:    []error{scaleway.ErrDecode},
			wantQuoted: []string{"bogusOfferingField"},
		},
		{
			name:    "trailing data after the object",
			data:    validDocument + "{}",
			wantErr: []error{scaleway.ErrDecode},
		},
		{
			name:    "unsupported schema version",
			data:    doc(t, `"schemaVersion": 1`, `"schemaVersion": 2`),
			wantErr: []error{scaleway.ErrSchemaVersion},
		},
		{
			name:    "missing schema version",
			data:    doc(t, "  \"schemaVersion\": 1,\n", ""),
			wantErr: []error{scaleway.ErrSchemaVersion},
		},
		{
			// Every declared region is a promise the file keeps: an offering in a
			// region the header does not declare would reach a report claiming a
			// geography the snapshot was never checked for.
			name:       "offering region not in the declared regions",
			data:       doc(t, `      "regions": ["fr-par"]`, `      "regions": ["fr-par", "xx-yy"]`),
			wantErr:    []error{scaleway.ErrUndeclaredRegion},
			wantQuoted: []string{"Instances", "xx-yy"},
		},
		{
			name:       "unknown capability",
			data:       doc(t, `"capabilities": ["ipv6"]`, `"capabilities": ["bogus-capability"]`),
			wantErr:    []error{taxonomy.ErrUnknownCapability},
			wantQuoted: []string{"bogus-capability"},
		},
		{
			name:    "retrieval date that is not a date",
			data:    doc(t, `"retrievedOn": "2026-11-10"`, `"retrievedOn": "2026-13-01"`),
			wantErr: []error{scaleway.ErrDecode},
		},
		{
			// The domain rejects a source URL that is not https, without echoing
			// it: a URL may carry credentials.
			name:    "source URL that is not https",
			data:    doc(t, "https://www.scaleway.com/en/docs/", "http://www.scaleway.com/en/docs/"),
			wantErr: []error{catalog.ErrInvalidCatalog},
		},
		{
			// A document of a version this package does not read is refused for
			// its version alone: a field only that version defines is no reason
			// to report it as unreadable.
			name: "schema version 2 carrying a field version 1 lacks",
			data: doc(t,
				`"schemaVersion": 1`, `"schemaVersion": 2`,
				`  "target": "Scaleway",`,
				"  \"target\": \"Scaleway\",\n  \"account\": \"123456789012\","),
			wantErr:    []error{scaleway.ErrSchemaVersion},
			wantNotErr: []error{scaleway.ErrDecode},
		},
		{
			// The version is a JSON number: a quoted number is another value, and
			// naming it must not quote the quotes.
			name:         "schema version as a string",
			data:         doc(t, `"schemaVersion": 1`, `"schemaVersion": "1"`),
			wantErr:      []error{scaleway.ErrSchemaVersion},
			wantContains: []string{"string", "\"1\""},
			wantAbsent:   []string{`\"`},
		},
		{
			// A field of the wrong type names the field, not the decoder's text.
			name:       "field of the wrong type",
			data:       doc(t, `"target": "Scaleway"`, `"target": 5`),
			wantErr:    []error{scaleway.ErrDecode},
			wantQuoted: []string{"target"},
		},
		{
			name:    "top-level array",
			data:    `[{"schemaVersion": 1}]`,
			wantErr: []error{scaleway.ErrDecode},
		},
		{
			name:    "top-level string",
			data:    `"a catalog"`,
			wantErr: []error{scaleway.ErrDecode},
		},
		{
			// null is a document of no schema version at all, not a document of
			// an unsupported one: the version check never runs.
			name:       "top-level null",
			data:       "null",
			wantErr:    []error{scaleway.ErrDecode},
			wantNotErr: []error{scaleway.ErrSchemaVersion},
		},
		{
			name:    "missing retrieval date",
			data:    doc(t, "  \"retrievedOn\": \"2026-11-10\",\n", ""),
			wantErr: []error{scaleway.ErrDecode},
		},
		{
			// The two header keys a catalog cannot exist without: a document
			// that omits them describes no offerings or declares no regions,
			// which is a mistake in the document, not an empty catalog.
			name:    "missing offerings key",
			data:    without(t, "offerings"),
			wantErr: []error{scaleway.ErrDecode},
		},
		{
			name:    "missing regions key",
			data:    without(t, "regions"),
			wantErr: []error{scaleway.ErrDecode},
		},
		{
			// Every declared region is a promise too: an entry that is no region
			// name would be declared for nothing. No offering uses the broken
			// entry, so the declaration itself is what has to be checked.
			name:       "declared region that is empty",
			data:       doc(t, `  "regions": ["fr-par"],`, `  "regions": ["fr-par", ""],`),
			wantErr:    []error{scaleway.ErrDecode},
			wantQuoted: []string{""},
		},
		{
			name:       "declared region that is only white space",
			data:       doc(t, `  "regions": ["fr-par"],`, `  "regions": ["fr-par", " "],`),
			wantErr:    []error{scaleway.ErrDecode},
			wantQuoted: []string{" "},
		},
		{
			name:       "declared region that is listed twice",
			data:       doc(t, `  "regions": ["fr-par"],`, `  "regions": ["fr-par", "fr-par"],`),
			wantErr:    []error{scaleway.ErrDecode},
			wantQuoted: []string{"fr-par"},
		},
		{
			name:       "declared region with a control character",
			data:       doc(t, `  "regions": ["fr-par"],`, "  \"regions\": [\"fr-par\", \"bad\\u0000x\"],"),
			wantErr:    []error{scaleway.ErrDecode},
			wantQuoted: []string{"bad\x00x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := scaleway.Parse([]byte(tt.data))

			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("Parse() returned unexpected error: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("Parse() returned no error, want one wrapping %v", tt.wantErr)
			}

			for _, sentinel := range tt.wantErr {
				if !errors.Is(err, sentinel) {
					t.Errorf("Parse() error = %v, want it to wrap %v", err, sentinel)
				}
			}

			for _, sentinel := range tt.wantNotErr {
				if errors.Is(err, sentinel) {
					t.Errorf("Parse() error = %v, want it not to wrap %v", err, sentinel)
				}
			}

			msg := err.Error()

			if strings.ContainsAny(msg, "\r\n") {
				t.Errorf("Parse() error = %q, want a single line", msg)
			}

			if errors.Is(err, scaleway.ErrDecode) {
				// The decoder's own text says where it gave up, in its terms, and
				// is not the package's error contract: the message describes the
				// problem in the package's words.
				for _, decoderText := range []string{"json:", "cannot unmarshal", "invalid character"} {
					if strings.Contains(msg, decoderText) {
						t.Errorf("Parse() error = %q, want it without the decoder's %s",
							msg, text.Quote(decoderText))
					}
				}
			}

			for _, value := range tt.wantQuoted {
				if quoted := text.Quote(value); !strings.Contains(msg, quoted) {
					t.Errorf("Parse() error = %q, want it to name %s through text.Quote", msg, quoted)
				}
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(msg, want) {
					t.Errorf("Parse() error = %q, want it to contain %s", msg, text.Quote(want))
				}
			}

			for _, unwanted := range tt.wantAbsent {
				if strings.Contains(msg, unwanted) {
					t.Errorf("Parse() error = %q, want it without %s", msg, text.Quote(unwanted))
				}
			}
		})
	}
}

// TestParseRejectsDocumentsOverTheSizeCap pins the size limit on a catalog
// document: a file over scaleway.MaxDocumentBytes is refused before it is decoded,
// and a file of exactly the limit is not, so the boundary is not off by one.
func TestParseRejectsDocumentsOverTheSizeCap(t *testing.T) {
	t.Parallel()

	const wantCap = 1 << 20
	if scaleway.MaxDocumentBytes != wantCap {
		t.Fatalf("MaxDocumentBytes = %d, want %d", scaleway.MaxDocumentBytes, wantCap)
	}

	atCap := padTo(t, validDocument, scaleway.MaxDocumentBytes)
	if _, err := scaleway.Parse(atCap); err != nil {
		t.Fatalf("Parse() of a document of exactly %d bytes returned unexpected error: %v",
			scaleway.MaxDocumentBytes, err)
	}

	overCap := padTo(t, validDocument, scaleway.MaxDocumentBytes+1)

	_, err := scaleway.Parse(overCap)
	if !errors.Is(err, scaleway.ErrDecode) {
		t.Fatalf("Parse() of a document of %d bytes returned error %v, want one wrapping %v",
			len(overCap), err, scaleway.ErrDecode)
	}

	if strings.ContainsAny(err.Error(), "\r\n") {
		t.Errorf("Parse() error = %q, want a single line", err.Error())
	}
}

// repeatedRegionDocument builds a catalog document that declares fifty regions
// and mentions the first of them 200 000 times, far more often than a real
// catalog does. Fifty distinct two-letter region names keep the document under
// the size cap even with every mention spelled out.
func repeatedRegionDocument(tb testing.TB) []byte {
	tb.Helper()

	const (
		declaredCount = 50
		mentions      = 200_000
	)

	regions := make([]string, declaredCount)
	for i := range regions {
		regions[i] = fmt.Sprintf(`"%c%c"`, 'a'+i/5, 'a'+i%5)
	}

	repeated := make([]string, mentions)
	for i := range repeated {
		repeated[i] = regions[0]
	}

	data := fmt.Sprintf(`{
  "schemaVersion": 1,
  "target": "Scaleway",
  "sourceURL": "https://www.scaleway.com/en/docs/",
  "retrievedOn": "2026-11-10",
  "regions": [%s],
  "offerings": [
    {
      "name": "Instances",
      "categories": ["compute.vm"],
      "capabilities": ["ipv6"],
      "regions": [%s]
    }
  ]
}`, strings.Join(regions, ", "), strings.Join(repeated, ","))

	if len(data) > scaleway.MaxDocumentBytes {
		tb.Fatalf("the document is %d bytes, over MaxDocumentBytes = %d: shorten the region names",
			len(data), scaleway.MaxDocumentBytes)
	}

	return []byte(data)
}

// BenchmarkParseRepeatedRegionMention measures parsing a document that mentions
// one declared region 200 000 times. It is a benchmark, not a test with a
// wall-clock budget: a single parse is far too quick to assert on without
// flaking on a loaded machine even at a generous margin, whereas the ns/op it
// reports exposes the regression it guards - a lookup that scans the declared
// regions once per mention instead of validating them once.
func BenchmarkParseRepeatedRegionMention(b *testing.B) {
	data := repeatedRegionDocument(b)

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))

	for b.Loop() {
		if _, err := scaleway.Parse(data); err != nil {
			b.Fatalf("Parse() of a %d-byte document mentioning a declared region 200 000 times returned unexpected error: %v",
				len(data), err)
		}
	}
}

// TestCatalogHonoursCancelledContext pins that a cancelled run reads nothing.
func TestCatalogHonoursCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := scaleway.New().Catalog(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Catalog() with a cancelled context returned error %v, want one wrapping %v", err, context.Canceled)
	}

	if strings.ContainsAny(err.Error(), "\r\n") {
		t.Errorf("Catalog() error = %q, want a single line", err.Error())
	}
}
