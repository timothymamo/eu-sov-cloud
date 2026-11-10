// Package snapshot_test pins the shared target snapshot reader: the document
// format every target with an embedded snapshot reads (schemaVersion 1), the
// strictness a document must meet, the declared region list Parse returns, and
// the reader's own error contract - one line, the sentinel it wraps, every value
// shown through text.Quote, the keys a document must carry and spell exactly,
// the nulls and duplicate keys it must refuse rather than default, the lone
// surrogates it must refuse rather than read as U+FFFD, the byte order marks
// (UTF-8 and UTF-16) it refuses by name, the line and column it places each
// problem at, the declared regions it must declare once and as valid text (UTF-8
// included), and the size limit above which a document is refused undecoded.
// Errors are worded for every target: the reader is shared, so no message names
// one. It also pins that every offering of every embedded snapshot has a section
// in the SOURCES.md beside it (see sources_test.go). It is black-box: only the
// package's exported API is used.
package snapshot_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/snapshot"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// validDocument is a complete, minimal catalog document: one offering in one
// declared region. The error table replaces one line of it to break exactly one
// thing. It uses a made-up target and domain, so no case can pass by agreeing
// with one provider's snapshot.
const validDocument = `{
  "schemaVersion": 1,
  "target": "Example Cloud",
  "sourceURL": "https://www.example.com/docs/",
  "retrievedOn": "2026-11-10",
  "regions": ["fr-par"],
  "offerings": [
    {
      "name": "Example Compute",
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

// topFields returns validDocument's top-level keys and their raw values, so a
// case can change or remove one without touching the rest. Re-encoding a map of
// raw fields keeps every nested value as it is; only the order of the top-level
// keys changes, which no test here depends on.
func topFields(t *testing.T) map[string]json.RawMessage {
	t.Helper()

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(validDocument), &fields); err != nil {
		t.Fatalf("unmarshalling validDocument: %v", err)
	}

	return fields
}

// marshalFields renders the top-level fields back as a document.
func marshalFields(t *testing.T, fields map[string]json.RawMessage) string {
	t.Helper()

	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshalling the document's top-level fields: %v", err)
	}

	return string(data)
}

// without returns validDocument with the top-level key removed, so a case pins a
// key the format requires.
func without(t *testing.T, key string) string {
	t.Helper()

	fields := topFields(t)

	if _, ok := fields[key]; !ok {
		t.Fatalf("without(): validDocument has no %s key", text.Quote(key))
	}

	delete(fields, key)

	return marshalFields(t, fields)
}

// with returns validDocument with the top-level key set to the raw JSON value,
// so a case pins a value the whole key takes, such as null.
func with(t *testing.T, key, raw string) string {
	t.Helper()

	fields := topFields(t)

	if _, ok := fields[key]; !ok {
		t.Fatalf("with(): validDocument has no %s key", text.Quote(key))
	}

	fields[key] = json.RawMessage(raw)

	return marshalFields(t, fields)
}

// offeringDoc returns validDocument with edit applied to the raw fields of its
// one offering, so a case changes or removes exactly one offering key and leaves
// the header and every other offering key valid.
func offeringDoc(t *testing.T, edit func(offering map[string]json.RawMessage)) string {
	t.Helper()

	fields := topFields(t)

	var offerings []map[string]json.RawMessage
	if err := json.Unmarshal(fields["offerings"], &offerings); err != nil {
		t.Fatalf("unmarshalling validDocument's offerings: %v", err)
	}

	if len(offerings) != 1 {
		t.Fatalf("validDocument has %d offerings, want 1", len(offerings))
	}

	edit(offerings[0])

	raw, err := json.Marshal(offerings)
	if err != nil {
		t.Fatalf("marshalling the offering's fields: %v", err)
	}

	fields["offerings"] = raw

	return marshalFields(t, fields)
}

// withOffering returns validDocument with the offering's key set to the raw JSON
// value, so a case pins a value the whole key takes, such as null.
func withOffering(t *testing.T, key, raw string) string {
	t.Helper()

	return offeringDoc(t, func(offering map[string]json.RawMessage) {
		if _, ok := offering[key]; !ok {
			t.Fatalf("withOffering(): validDocument's offering has no %s key", text.Quote(key))
		}

		offering[key] = json.RawMessage(raw)
	})
}

// withoutOffering returns validDocument with the offering's key removed, so a
// case pins an offering key the format requires.
func withoutOffering(t *testing.T, key string) string {
	t.Helper()

	return offeringDoc(t, func(offering map[string]json.RawMessage) {
		if _, ok := offering[key]; !ok {
			t.Fatalf("withoutOffering(): validDocument's offering has no %s key", text.Quote(key))
		}

		delete(offering, key)
	})
}

// TestParseErrors pins every way parsing a catalog document fails: the sentinel
// the error wraps, and that the message is one line naming the offending value
// through text.Quote. A document the reader cannot understand is never silently
// dropped or defaulted: an offering in a region the file does not declare or
// with an unknown capability must fail, and so must a key given twice, a key
// spelt differently from the format, a null or missing field, and a string with
// an escaped lone surrogate, which the JSON decoder on its own would read as
// U+FFFD.
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
		// wantPosition is where the message must place the problem: a 1-based
		// line and column in the document, never a byte offset. It is counted
		// from the document's own text, in characters, so a multi-byte one
		// counts once.
		wantPosition string
	}{
		{
			name: "valid document",
			data: validDocument,
		},
		{
			name:    "not JSON",
			data:    "not json",
			wantErr: []error{snapshot.ErrDecode},
		},
		{
			name: "unknown top-level field",
			data: doc(t, `  "schemaVersion": 1,`,
				"  \"schemaVersion\": 1,\n  \"bogusField\": true,"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"bogusField"},
			wantContains: []string{"unknown key"},
		},
		{
			name: "unknown offering field",
			data: doc(t, `      "name": "Example Compute",`,
				"      \"name\": \"Example Compute\",\n      \"bogusOfferingField\": \"x\","),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"bogusOfferingField"},
			wantContains: []string{"unknown key"},
		},
		{
			// encoding/json keeps the last of two equal keys, so without a check
			// the first value would be dropped without a word.
			name: "top-level key given twice",
			data: doc(t, `  "target": "Example Cloud",`,
				"  \"target\": \"Example Cloud\",\n  \"target\": \"Example Cloud\","),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"target"},
			wantContains: []string{"repeated key"},
			// The second of the two lines is the one refused, and its key
			// starts after the two spaces that indent it.
			wantPosition: "line 4, column 3",
		},
		{
			name: "offering key given twice",
			data: doc(t, `      "regions": ["fr-par"]`,
				"      \"regions\": [\"fr-par\"],\n      \"regions\": [\"fr-par\"]"),
			wantErr:    []error{snapshot.ErrDecode},
			wantQuoted: []string{"offerings[0].regions"},
			// The repeated line is the offering's last one, indented six
			// spaces, so its key starts in column 7.
			wantContains: []string{"repeated key"},
			wantPosition: "line 13, column 7",
		},
		{
			// encoding/json matches field names without regard to case, so a key
			// spelt differently from the format would be read as the right one
			// instead of being refused.
			name:         "top-level key not spelt as the format spells it",
			data:         doc(t, `  "regions": ["fr-par"],`, `  "Regions": ["fr-par"],`),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"Regions", "regions"},
			wantContains: []string{"unknown key", "keys are exact"},
		},
		{
			name: "top-level URL key not spelt as the format spells it",
			data: doc(t, `  "sourceURL": "https://www.example.com/docs/",`,
				`  "sourceUrl": "https://www.example.com/docs/",`),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"sourceUrl", "sourceURL"},
			wantContains: []string{"unknown key", "keys are exact"},
		},
		{
			name: "offering key not spelt as the format spells it",
			data: doc(t, `      "name": "Example Compute",`,
				"      \"name\": \"Example Compute\",\n      \"Name\": \"Example Compute\","),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings[0].Name", "name"},
			wantContains: []string{"unknown key", "keys are exact"},
		},
		{
			// What follows is described by what it follows: the first JSON value,
			// whatever its type, not an object the document never had to be. The
			// {} starts on the document's last line, just after the object's brace.
			name:         "trailing data after the object",
			data:         validDocument + "{}",
			wantErr:      []error{snapshot.ErrDecode},
			wantContains: []string{"unexpected data after the first JSON value"},
			wantPosition: "line 15, column 2",
			wantAbsent:   []string{"catalog object"},
		},
		{
			// The first value need not be an object for the reader to say what
			// comes after it: "a" is the first JSON value, and the second one
			// starts after the single space between them.
			name:         "first value that is not an object followed by more data",
			data:         `"a" "b"`,
			wantErr:      []error{snapshot.ErrDecode},
			wantContains: []string{"unexpected data after the first JSON value"},
			wantPosition: "line 1, column 5",
			wantAbsent:   []string{"catalog object"},
		},
		{
			name:    "unsupported schema version",
			data:    doc(t, `"schemaVersion": 1`, `"schemaVersion": 2`),
			wantErr: []error{snapshot.ErrSchemaVersion},
		},
		{
			name:    "missing schema version",
			data:    doc(t, "  \"schemaVersion\": 1,\n", ""),
			wantErr: []error{snapshot.ErrSchemaVersion},
		},
		{
			// encoding/json would read SchemaVersion as schemaVersion. The version
			// is refused for itself alone, and the message names the key the
			// writer spelt so the mistake is visible.
			name:         "schema version key not spelt as the format spells it",
			data:         doc(t, `"schemaVersion": 1`, `"SchemaVersion": 1`),
			wantErr:      []error{snapshot.ErrSchemaVersion},
			wantNotErr:   []error{snapshot.ErrDecode},
			wantQuoted:   []string{"SchemaVersion"},
			wantContains: []string{"keys are exact", "is not schemaVersion"},
		},
		{
			// Every declared region is a promise the file keeps: an offering in a
			// region the header does not declare would reach a report claiming a
			// geography the snapshot was never checked for.
			name:       "offering region not in the declared regions",
			data:       doc(t, `      "regions": ["fr-par"]`, `      "regions": ["fr-par", "xx-yy"]`),
			wantErr:    []error{snapshot.ErrUndeclaredRegion},
			wantQuoted: []string{"Example Compute", "xx-yy"},
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
			wantErr: []error{snapshot.ErrDecode},
		},
		{
			// The domain rejects a source URL that is not https, without echoing
			// it: a URL may carry credentials.
			name:    "source URL that is not https",
			data:    doc(t, "https://www.example.com/docs/", "http://www.example.com/docs/"),
			wantErr: []error{catalog.ErrInvalidCatalog},
		},
		{
			// A document of a version this package does not read is refused for
			// its version alone: a field only that version defines is no reason
			// to report it as unreadable.
			name: "schema version 2 carrying a field version 1 lacks",
			data: doc(t,
				`"schemaVersion": 1`, `"schemaVersion": 2`,
				`  "target": "Example Cloud",`,
				"  \"target\": \"Example Cloud\",\n  \"account\": \"123456789012\","),
			wantErr:    []error{snapshot.ErrSchemaVersion},
			wantNotErr: []error{snapshot.ErrDecode},
		},
		{
			// A duplicate key in a document of another version is the version's
			// fault alone for the same reason: the strictness checks never run.
			name: "schema version 2 with a key given twice",
			data: doc(t,
				`"schemaVersion": 1`, `"schemaVersion": 2`,
				`  "target": "Example Cloud",`,
				"  \"target\": \"Example Cloud\",\n  \"target\": \"Example Cloud\","),
			wantErr:    []error{snapshot.ErrSchemaVersion},
			wantNotErr: []error{snapshot.ErrDecode},
		},
		{
			// The version is a JSON number: a quoted number is another value, and
			// naming it must not quote the quotes.
			name:         "schema version as a string",
			data:         doc(t, `"schemaVersion": 1`, `"schemaVersion": "1"`),
			wantErr:      []error{snapshot.ErrSchemaVersion},
			wantContains: []string{"string", "\"1\""},
			wantAbsent:   []string{`\"`},
		},
		{
			// A field of the wrong type names the field, not the decoder's text.
			name:         "field of the wrong type",
			data:         doc(t, `"target": "Example Cloud"`, `"target": 5`),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"target"},
			wantContains: []string{"is a number", "want a string"},
		},
		{
			name:    "top-level array",
			data:    `[{"schemaVersion": 1}]`,
			wantErr: []error{snapshot.ErrDecode},
		},
		{
			name:    "top-level string",
			data:    `"a catalog"`,
			wantErr: []error{snapshot.ErrDecode},
		},
		{
			// null is a document of no schema version at all, not a document of
			// an unsupported one: the version check never runs.
			name:       "top-level null",
			data:       "null",
			wantErr:    []error{snapshot.ErrDecode},
			wantNotErr: []error{snapshot.ErrSchemaVersion},
		},
		{
			name:    "missing retrieval date",
			data:    doc(t, "  \"retrievedOn\": \"2026-11-10\",\n", ""),
			wantErr: []error{snapshot.ErrDecode},
		},
		{
			// The two header keys a catalog cannot exist without: a document
			// that omits them describes no offerings or declares no regions,
			// which is a mistake in the document, not an empty catalog.
			name:         "missing offerings key",
			data:         without(t, "offerings"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings"},
			wantContains: []string{"required key", "is missing"},
		},
		{
			name:         "missing regions key",
			data:         without(t, "regions"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"regions"},
			wantContains: []string{"required key", "is missing"},
		},
		{
			// A header field the document leaves out says nothing: read as the
			// empty value, it would be a document the reader silently filled in.
			name:         "missing target",
			data:         without(t, "target"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"target"},
			wantContains: []string{"required key", "is missing"},
		},
		{
			name:         "missing source URL",
			data:         without(t, "sourceURL"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"sourceURL"},
			wantContains: []string{"required key", "is missing"},
		},
		{
			// null is not an empty value either: a field given as null would be
			// read as the zero value, which is a value the document never said.
			name:         "null target",
			data:         with(t, "target", "null"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"target"},
			wantContains: []string{"is null", "want a string", "at line ", ", column "},
		},
		{
			name:         "null source URL",
			data:         with(t, "sourceURL", "null"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"sourceURL"},
			wantContains: []string{"is null", "want a string"},
		},
		{
			name:         "null retrieval date",
			data:         with(t, "retrievedOn", "null"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"retrievedOn"},
			wantContains: []string{"is null", "want a string"},
		},
		{
			// null is not an empty list: a header that says regions: null
			// says nothing it means to say, which is a mistake in the document.
			name:         "null regions list",
			data:         with(t, "regions", "null"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"regions"},
			wantContains: []string{"is null", "want an array of strings"},
		},
		{
			name:         "null offerings list",
			data:         with(t, "offerings", "null"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings"},
			wantContains: []string{"is null", "want an array of objects"},
		},
		{
			name:         "missing offering name",
			data:         withoutOffering(t, "name"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings[0]"},
			wantContains: []string{"required key", "is missing"},
		},
		{
			name:         "missing offering categories",
			data:         withoutOffering(t, "categories"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings[0]"},
			wantContains: []string{"required key", "is missing"},
		},
		{
			// catalog.New accepts an offering with no capabilities, so without
			// this check a document that forgot the key would read as an offering
			// with none: silently defaulted. The message says which offering it is
			// by where that offering starts, the brace on line 8 in column 5.
			name:         "missing offering capabilities",
			data:         doc(t, "      \"capabilities\": [\"ipv6\"],\n", ""),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings[0]"},
			wantContains: []string{"required key", "is missing", "which starts at line 8, column 5"},
			wantPosition: "line 8, column 5",
		},
		{
			name:         "missing offering regions",
			data:         withoutOffering(t, "regions"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings[0]"},
			wantContains: []string{"required key", "is missing"},
		},
		{
			name:         "null offering name",
			data:         doc(t, `      "name": "Example Compute",`, `      "name": null,`),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings[0].name"},
			wantContains: []string{"is null", "want a string"},
			// null is placed where it is written: after the six spaces, the
			// quoted key and the colon, which leaves column 15.
			wantPosition: "line 9, column 15",
		},
		{
			name:         "null offering categories",
			data:         withOffering(t, "categories", "null"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings[0].categories"},
			wantContains: []string{"is null", "want an array of strings"},
		},
		{
			name:         "null offering capabilities",
			data:         withOffering(t, "capabilities", "null"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings[0].capabilities"},
			wantContains: []string{"is null", "want an array of strings"},
		},
		{
			name:         "null offering regions",
			data:         withOffering(t, "regions", "null"),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings[0].regions"},
			wantContains: []string{"is null", "want an array of strings"},
		},
		{
			// An explicit empty list is a value the document stated, and an
			// offering with no capability is a real offering: it is accepted.
			name: "offering with an empty capabilities list",
			data: withOffering(t, "capabilities", "[]"),
		},
		{
			// encoding/json reads an escaped lone surrogate as U+FFFD and reports
			// nothing, so the name would reach a report as a different string
			// from the one the document wrote.
			name:         "escaped lone high surrogate in an offering name",
			data:         doc(t, `"name": "Example Compute"`, `"name": "\ud800"`),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings[0].name"},
			wantContains: []string{"surrogate on its own", "U+D800", "at line ", ", column "},
		},
		{
			name:         "escaped lone low surrogate in the target",
			data:         doc(t, `"target": "Example Cloud"`, `"target": "\udc00"`),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"target"},
			wantContains: []string{"surrogate on its own", "U+DC00"},
		},
		{
			// The scan of the raw text looks only for lone surrogates: a string
			// whose author wrote U+FFFD on purpose is text like any other, and
			// refusing it would refuse a document the format allows.
			name: "offering name holding U+FFFD",
			data: doc(t, `"name": "Example Compute"`, `"name": "Example\ufffdCompute"`),
		},
		{
			// The name is U+FFFD, then an escaped backslash, then the text ud800.
			// The JSON text \\ud800 is a backslash and four letters, no escape of
			// its own: a scan that read the second backslash as the start of an
			// escape would refuse a document the format allows.
			name: "offering name holding U+FFFD before an escaped backslash and ud800",
			data: doc(t, `      "name": "Example Compute",`, `      "name": "\ufffd\\ud800",`),
		},
		{
			// A lone surrogate is placed at the escape that spells it, counted in
			// characters from the start of the line: after the two-byte é, the
			// backslash of \ud800 is line 9's character column 17.
			name:         "escaped lone surrogate after a multi-byte character",
			data:         doc(t, `      "name": "Example Compute",`, `      "name": "é\ud800",`),
			wantErr:      []error{snapshot.ErrDecode},
			wantQuoted:   []string{"offerings[0].name"},
			wantContains: []string{"surrogate on its own", "U+D800"},
			wantPosition: "line 9, column 17",
		},
		{
			// A surrogate pair is one character, not two lone surrogates: the
			// high one is followed at once by the low one.
			name: "offering name holding a surrogate pair",
			data: doc(t, `"name": "Example Compute"`, `"name": "Example \ud83d\ude00"`),
		},
		{
			// Every declared region is a promise too: an entry that is no region
			// name would be declared for nothing. No offering uses the broken
			// entry, so the declaration itself is what has to be checked.
			name:       "declared region that is empty",
			data:       doc(t, `  "regions": ["fr-par"],`, `  "regions": ["fr-par", ""],`),
			wantErr:    []error{snapshot.ErrDecode},
			wantQuoted: []string{""},
		},
		{
			name:       "declared region that is only white space",
			data:       doc(t, `  "regions": ["fr-par"],`, `  "regions": ["fr-par", " "],`),
			wantErr:    []error{snapshot.ErrDecode},
			wantQuoted: []string{" "},
		},
		{
			name:       "declared region that is listed twice",
			data:       doc(t, `  "regions": ["fr-par"],`, `  "regions": ["fr-par", "fr-par"],`),
			wantErr:    []error{snapshot.ErrDecode},
			wantQuoted: []string{"fr-par"},
		},
		{
			name:       "declared region with a control character",
			data:       doc(t, `  "regions": ["fr-par"],`, "  \"regions\": [\"fr-par\", \"bad\\u0000x\"],"),
			wantErr:    []error{snapshot.ErrDecode},
			wantQuoted: []string{"bad\x00x"},
		},
		{
			// A region name is text that reaches reports and terminals, and a
			// byte that is not UTF-8 has no text to show there. The JSON decoder
			// on its own would turn it into U+FFFD and report nothing, so the
			// document's own bytes have to be checked.
			name:         "declared region with an invalid UTF-8 byte",
			data:         doc(t, `  "regions": ["fr-par"],`, "  \"regions\": [\"fr-par\", \"bad\xffx\"],"),
			wantErr:      []error{snapshot.ErrDecode},
			wantContains: []string{"UTF-8"},
		},
		{
			// An invalid byte is refused where it is, not at the start of the
			// document: the FF of the offering name is line 9's character column
			// 19, which is where the name's fourth character begins.
			name:         "invalid UTF-8 in an offering name",
			data:         doc(t, `      "name": "Example Compute",`, "      \"name\": \"bad\xffname\","),
			wantErr:      []error{snapshot.ErrDecode},
			wantContains: []string{"not valid UTF-8"},
			wantPosition: "line 9, column 19",
			wantAbsent:   []string{"line 1, column 1"},
		},
		{
			// A byte order mark is not JSON, and the UTF-8 one is valid UTF-8, so
			// only naming it stops the reader reporting it as malformed JSON at
			// the first byte.
			name:         "UTF-8 byte order mark",
			data:         "\xef\xbb\xbf" + validDocument,
			wantErr:      []error{snapshot.ErrDecode},
			wantContains: []string{"byte order mark"},
		},
		{
			// The UTF-16 byte order marks are not UTF-8 either, and are refused by
			// name like the UTF-8 one rather than as a byte that is not UTF-8.
			name:         "UTF-16LE byte order mark",
			data:         "\xff\xfe" + validDocument,
			wantErr:      []error{snapshot.ErrDecode},
			wantContains: []string{"byte order mark"},
			wantAbsent:   []string{"not valid UTF-8"},
		},
		{
			name:         "UTF-16BE byte order mark",
			data:         "\xfe\xff" + validDocument,
			wantErr:      []error{snapshot.ErrDecode},
			wantContains: []string{"byte order mark"},
			wantAbsent:   []string{"not valid UTF-8"},
		},
		{
			// A multi-byte character where the next key belongs is malformed JSON,
			// and the JSON decoder counts the rune's last byte: the message must
			// place the character at its first byte, which is line 3's column 2.
			name:         "malformed two-byte character where a key is expected",
			data:         "{\n \"schemaVersion\": 1,\n é\n}",
			wantErr:      []error{snapshot.ErrDecode},
			wantContains: []string{"malformed JSON"},
			wantPosition: "line 3, column 2",
			wantAbsent:   []string{"column 3"},
		},
		{
			// The same for a character of three bytes: the step back to the first
			// byte of the rune covers every byte of it.
			name:         "malformed three-byte character where a key is expected",
			data:         "{\n \"schemaVersion\": 1,\n €\n}",
			wantErr:      []error{snapshot.ErrDecode},
			wantContains: []string{"malformed JSON"},
			wantPosition: "line 3, column 2",
			wantAbsent:   []string{"column 4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := snapshot.Parse([]byte(tt.data))

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

			if errors.Is(err, snapshot.ErrDecode) {
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

			if tt.wantPosition != "" && !strings.Contains(msg, tt.wantPosition) {
				t.Errorf("Parse() error = %q, want it to report the position as %q", msg, tt.wantPosition)
			}
		})
	}
}

// TestParseErrorsNameNoTarget pins that the reader's errors are worded for every
// target: the reader is shared by every target adapter, so a message may not say
// which target's document it was reading. Each document here has the target
// "Example Cloud", and a failing one must name neither of the targets the
// project ships.
func TestParseErrorsNameNoTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{name: "not JSON", data: "not json"},
		{
			name: "duplicate top-level key",
			data: doc(t, `  "target": "Example Cloud",`,
				"  \"target\": \"Example Cloud\",\n  \"target\": \"Example Cloud\","),
		},
		{name: "null target", data: with(t, "target", "null")},
		{
			name: "escaped lone surrogate in the target",
			data: doc(t, `"target": "Example Cloud"`, `"target": "\ud800"`),
		},
		{
			name: "retrieval date that is not a date",
			data: doc(t, `"retrievedOn": "2026-11-10"`, `"retrievedOn": "yesterday"`),
		},
		{
			name: "offering region not in the declared regions",
			data: doc(t, `      "regions": ["fr-par"]`, `      "regions": ["xx-yy"]`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := snapshot.Parse([]byte(tt.data))
			if err == nil {
				t.Fatalf("Parse() of a document with target %s returned no error, want one",
					text.Quote("Example Cloud"))
			}

			for _, target := range []string{"Scaleway", "OVHcloud"} {
				if strings.Contains(err.Error(), target) {
					t.Errorf("Parse() error = %q, want it without the target name %s",
						err.Error(), text.Quote(target))
				}
			}
		})
	}
}

// TestParseRegions pins the declared region list Parse returns: the order the
// document lists the regions in, not a sorted one (callers report it as the
// snapshot declares it), and the caller's own copy, so changing one result
// cannot change what a later Parse returns.
func TestParseRegions(t *testing.T) {
	t.Parallel()

	// The offering lists the same regions in the same order, which the document
	// requires to be a subset of the declared ones.
	data := doc(t,
		`  "regions": ["fr-par"],`, `  "regions": ["pl-waw", "fr-par", "nl-ams"],`,
		`      "regions": ["fr-par"]`, `      "regions": ["pl-waw", "fr-par", "nl-ams"]`)

	want := []string{"pl-waw", "fr-par", "nl-ams"}

	document, err := snapshot.Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() returned unexpected error: %v", err)
	}

	if !slices.Equal(document.Regions, want) {
		t.Fatalf("Parse().Regions = %v, want the document's order %v", document.Regions, want)
	}

	for i := range document.Regions {
		document.Regions[i] = "tampered"
	}

	again, err := snapshot.Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() after changing the first result returned unexpected error: %v", err)
	}

	if !slices.Equal(again.Regions, want) {
		t.Errorf("Parse().Regions after changing the first result = %v, want %v: the result is not a copy",
			again.Regions, want)
	}
}

// TestParseRejectsDocumentsOverTheSizeCap pins the size limit on a catalog
// document: a file over snapshot.MaxDocumentBytes is refused before it is
// decoded, and a file of exactly the limit is not, so the boundary is not off by
// one.
func TestParseRejectsDocumentsOverTheSizeCap(t *testing.T) {
	t.Parallel()

	const wantCap = 1 << 20
	if snapshot.MaxDocumentBytes != wantCap {
		t.Fatalf("MaxDocumentBytes = %d, want %d", snapshot.MaxDocumentBytes, wantCap)
	}

	atCap := padTo(t, validDocument, snapshot.MaxDocumentBytes)
	if _, err := snapshot.Parse(atCap); err != nil {
		t.Fatalf("Parse() of a document of exactly %d bytes returned unexpected error: %v",
			snapshot.MaxDocumentBytes, err)
	}

	overCap := padTo(t, validDocument, snapshot.MaxDocumentBytes+1)

	_, err := snapshot.Parse(overCap)
	if !errors.Is(err, snapshot.ErrDecode) {
		t.Fatalf("Parse() of a document of %d bytes returned error %v, want one wrapping %v",
			len(overCap), err, snapshot.ErrDecode)
	}

	if strings.ContainsAny(err.Error(), "\r\n") {
		t.Errorf("Parse() error = %q, want a single line", err.Error())
	}
}

// emptyCatalogDocument is a complete catalog that declares no region and no
// offering. Both lists are present and empty, which is a catalog of nothing;
// null (see TestParseErrors) is a document that does not say.
const emptyCatalogDocument = `{
  "schemaVersion": 1,
  "target": "Example Cloud",
  "sourceURL": "https://www.example.com/docs/",
  "retrievedOn": "2026-11-10",
  "regions": [],
  "offerings": []
}`

// TestParseEmptyCatalog pins that a document declaring no region and no offering
// is an empty catalog, not a broken one: an empty list is an answer, which is why
// only null is refused. It is the other side of the two null rows above.
func TestParseEmptyCatalog(t *testing.T) {
	t.Parallel()

	document, err := snapshot.Parse([]byte(emptyCatalogDocument))
	if err != nil {
		t.Fatalf("Parse() of a document declaring no region and no offering returned unexpected error: %v", err)
	}

	if got := document.Catalog.Target(); got != "Example Cloud" {
		t.Errorf("Parse().Catalog.Target() = %s, want %s", text.Quote(got), text.Quote("Example Cloud"))
	}

	if got := document.Catalog.Offerings(); len(got) != 0 {
		t.Errorf("Parse().Catalog.Offerings() = %v, want none", got)
	}

	if len(document.Regions) != 0 {
		t.Errorf("Parse().Regions = %v, want none", document.Regions)
	}
}

// repeatedRegionDocument builds a catalog document that declares many short,
// distinct regions and mentions the last of them as often as the size cap allows.
//
// Mentioning the last declared region is what makes the document a worst case: a
// lookup that finds a region by scanning the declared list stops at the first
// match, so a mention of an early region hides the scan. Only the region furthest
// down the list, mentioned a hundred thousand times, makes the scan cost one
// comparison per declared region per mention.
func repeatedRegionDocument(tb testing.TB) []byte {
	tb.Helper()

	// Four hex digits name every region in five bytes quoted, so the declared
	// list stays a fraction of MaxDocumentBytes and leaves the rest for mentions.
	const declaredCount = 60_000

	regions := make([]string, declaredCount)
	for i := range regions {
		regions[i] = strconv.Quote(fmt.Sprintf("%04x", i))
	}

	last := regions[declaredCount-1]

	// compose renders the document around the offering's region mentions, so the
	// size of every other byte can be measured with the mentions empty.
	compose := func(mentions string) string {
		return fmt.Sprintf(`{
  "schemaVersion": 1,
  "target": "Example Cloud",
  "sourceURL": "https://www.example.com/docs/",
  "retrievedOn": "2026-11-10",
  "regions": [%s],
  "offerings": [
    {
      "name": "Example Compute",
      "categories": ["compute.vm"],
      "capabilities": ["ipv6"],
      "regions": [%s]
    }
  ]
}`, strings.Join(regions, ", "), mentions)
	}

	// Every mention but the last is followed by a comma, so a mention costs the
	// region's quoted length plus the comma.
	perMention := len(last) + len(",")

	mentions := (snapshot.MaxDocumentBytes - len(compose(""))) / perMention
	if mentions < 2 {
		tb.Fatalf("the declared regions leave room for %d mentions, want at least 2, under MaxDocumentBytes = %d",
			mentions, snapshot.MaxDocumentBytes)
	}

	data := compose(strings.TrimSuffix(strings.Repeat(last+",", mentions), ","))

	if len(data) > snapshot.MaxDocumentBytes {
		tb.Fatalf("the document is %d bytes, over MaxDocumentBytes = %d: shorten the region names",
			len(data), snapshot.MaxDocumentBytes)
	}

	return []byte(data)
}

// TestParseRepeatedRegionMention parses the worst-case document once, so the
// default test run exercises the builder and the size guard it works to. It
// asserts no duration: the cost is what BenchmarkParseRepeatedRegionMention
// reports, not something a wall-clock budget can pin without flaking.
func TestParseRepeatedRegionMention(t *testing.T) {
	t.Parallel()

	data := repeatedRegionDocument(t)

	if _, err := snapshot.Parse(data); err != nil {
		t.Fatalf("Parse() of the %d-byte document that mentions its last declared region as often as the size cap allows returned unexpected error: %v",
			len(data), err)
	}
}

// BenchmarkParseRepeatedRegionMention measures parsing a document that declares
// 60 000 regions and mentions the last of them as often as MaxDocumentBytes
// allows. It is a benchmark, not a test with a wall-clock budget: a single parse
// is far too quick to assert on without flaking on a loaded machine even at a
// generous margin, whereas the ns/op it reports exposes the regression it guards -
// a lookup that scans the declared regions once per mention instead of validating
// them once. Mentioning the last of many regions is what makes the scan visible:
// a mention of the first region is found on the first comparison.
func BenchmarkParseRepeatedRegionMention(b *testing.B) {
	data := repeatedRegionDocument(b)

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))

	for b.Loop() {
		if _, err := snapshot.Parse(data); err != nil {
			b.Fatalf("Parse() of a %d-byte document mentioning its last declared region as often as the size cap allows returned unexpected error: %v",
				len(data), err)
		}
	}
}
