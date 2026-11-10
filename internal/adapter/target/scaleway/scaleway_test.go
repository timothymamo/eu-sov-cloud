// Package scaleway_test pins the Scaleway catalog adapter: the target contract in
// internal/adapter/target/targettest, the snapshot header the catalog reports, a
// golden change detector for the parsed catalog, the declared EU regions, and the
// adapter's own error contract - one line, the sentinel it wraps, every value
// shown through text.Quote. It is black-box: only the package's exported API is
// used.
package scaleway_test

import (
	"context"
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
		// wantQuoted lists values the message must show through text.Quote.
		wantQuoted []string
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

			msg := err.Error()

			if strings.ContainsAny(msg, "\r\n") {
				t.Errorf("Parse() error = %q, want a single line", msg)
			}

			for _, value := range tt.wantQuoted {
				if quoted := text.Quote(value); !strings.Contains(msg, quoted) {
					t.Errorf("Parse() error = %q, want it to name %s through text.Quote", msg, quoted)
				}
			}
		})
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
