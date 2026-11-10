// Package scaleway describes the Scaleway service catalog in domain terms. It
// is the driven adapter behind the analysis.CatalogSource port for the Scaleway
// target, and it passes the contract suite in internal/adapter/target/targettest.
//
// The catalog is an embedded, dated snapshot, catalog.json (//go:embed),
// compiled by hand from Scaleway's public documentation and SDK. The adapter
// makes no network calls and needs no credentials. SOURCES.md records where
// each offering, capability and region comes from.
//
// catalog.json is a JSON document of schema version 1: a header with the
// target's display name, the source URL and the retrieval date (retrievedOn,
// YYYY-MM-DD) of the data, the EU regions the snapshot declares, and the
// offerings, each with its taxonomy categories, capabilities and regions.
// Source.Catalog serves the embedded file through Parse, and Regions returns
// its declared region list.
//
// Parse refuses a document, with an error, unless all of these hold:
//
//   - it is at most MaxDocumentBytes long, checked before any decoding;
//   - it is one JSON object with nothing after it but white space;
//   - its schemaVersion is the integer 1, checked first, so a document of
//     another version is refused for its version alone;
//   - it has no field the format does not define, at the top level or in an
//     offering, and every field has the JSON type the format gives it;
//   - its regions and offerings lists are present and not null;
//   - its retrievedOn is a YYYY-MM-DD date;
//   - every declared region is non-blank, valid text (text.Validate) and
//     declared once, and every offering region is a declared one;
//   - catalog.New accepts its target, its source URL and every offering's
//     name, categories, capabilities and regions.
//
// Two things are not refused yet. encoding/json matches field names without
// regard to case and, of a key given twice, keeps the last value; and
// catalog.New sorts an offering's lists and drops duplicates in them. Refusing
// keys that are not spelled exactly, and duplicate keys, through one strict
// decoder shared with the inventory-file source adapter, is a tracked
// follow-up.
//
// To refresh the snapshot: edit catalog.json, update SOURCES.md and the
// retrievedOn date to match, run
//
//	go test ./internal/adapter/target/scaleway -update
//
// and review the diff of testdata/catalog.golden, which pins the parsed
// catalog, in the same change. A change to the declared regions also changes
// the region list the package's tests write out, on purpose: it is checked
// against Scaleway's published regions in review.
package scaleway
