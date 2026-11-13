package snapshot_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/internal/adapter/target/snapshot"
)

// repeatedPriceRegionDocument returns a document, under MaxDocumentBytes, whose
// one offering lists region "a" about as often as half the size cap allows and
// region "b" once, last, and whose one storage price lists "b" as often as the
// other half allows. Repetition carries no meaning (ADR 0007), so the document
// is valid.
//
// It is the worst case for checking a price region against its offering's
// regions: a lookup that scans the offering's regions as listed finds "b" only
// after every "a", once per mention of "b" in the price, so its cost is the
// product of the two lists' lengths.
func repeatedPriceRegionDocument(tb testing.TB) []byte {
	tb.Helper()

	compose := func(offering, price string) string {
		return fmt.Sprintf(`{
  "schemaVersion": 2,
  "target": "Example Cloud",
  "sourceURL": "https://www.example.com/docs/",
  "retrievedOn": "2026-11-10",
  "regions": ["a", "b"],
  "defaultRegion": "a",
  "offerings": [
    {
      "name": "Example Storage",
      "categories": ["storage.block"],
      "capabilities": [],
      "regions": [%s"b"],
      "plans": [],
      "storagePrices": [
        { "amount": 0.04, "currency": "EUR", "regions": [%s] }
      ]
    }
  ]
}`, offering, price)
	}

	// Each mention is a quoted one-letter region and a comma: four bytes.
	const perMention = len(`"a",`)

	room := snapshot.MaxDocumentBytes - len(compose("", `"b"`))
	half := room / 2 / perMention

	if half < 2 {
		tb.Fatalf("the document leaves room for %d mentions per list, want at least 2, under MaxDocumentBytes = %d",
			half, snapshot.MaxDocumentBytes)
	}

	data := compose(strings.Repeat(`"a",`, half), strings.Repeat(`"b",`, half)+`"b"`)

	if len(data) > snapshot.MaxDocumentBytes {
		tb.Fatalf("the document is %d bytes, over MaxDocumentBytes = %d", len(data), snapshot.MaxDocumentBytes)
	}

	return []byte(data)
}

// TestParseRepeatedPriceRegion parses the worst-case document once, so the
// default test run exercises its builder. It asserts no duration: the cost is
// what BenchmarkParseRepeatedPriceRegion reports.
func TestParseRepeatedPriceRegion(t *testing.T) {
	t.Parallel()

	data := repeatedPriceRegionDocument(t)

	if _, err := snapshot.Parse(data); err != nil {
		t.Fatalf("Parse() of the %d-byte document repeating its price and offering regions returned unexpected error: %v",
			len(data), err)
	}
}

// BenchmarkParseRepeatedPriceRegion measures parsing a document whose storage
// price and offering each repeat a region as often as MaxDocumentBytes allows.
// The ns/op it reports exposes the regression it guards: a price-region check
// that scans the offering's regions once per price region instead of looking
// them up in a set (ADR 0024).
func BenchmarkParseRepeatedPriceRegion(b *testing.B) {
	data := repeatedPriceRegionDocument(b)

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))

	for b.Loop() {
		if _, err := snapshot.Parse(data); err != nil {
			b.Fatalf("Parse() of the %d-byte document repeating its price and offering regions returned unexpected error: %v",
				len(data), err)
		}
	}
}
