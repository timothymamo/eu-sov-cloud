package snapshot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// schemaVersion is the only version of the snapshot format this package reads
// (ADR 0007).
const schemaVersion = 1

// MaxDocumentBytes is the size of the largest snapshot Parse reads, in bytes. A
// larger one is refused before any of it is decoded. An embedded snapshot is a
// few kilobytes; the limit leaves room to grow while keeping a document that is
// not a snapshot from costing more than a moment to refuse.
const MaxDocumentBytes = 1 << 20

var (
	// ErrDecode is returned for data that is not a well-formed snapshot: larger
	// than MaxDocumentBytes, not valid UTF-8, not one JSON object, a key that is
	// unknown, not spelt exactly or repeated, a field that is missing, null or of
	// the wrong JSON type, a string that escapes a lone surrogate, a retrieval
	// date that is not a YYYY-MM-DD date, or a declared region that is blank, not
	// valid text (text.Validate) or declared twice.
	ErrDecode = errors.New("not a valid catalog document")
	// ErrSchemaVersion is returned for a document whose schemaVersion is missing
	// or is not a version this package reads.
	ErrSchemaVersion = errors.New("unsupported schema version")
	// ErrUndeclaredRegion is returned for an offering available in a region the
	// document's top-level regions list does not declare.
	ErrUndeclaredRegion = errors.New("offering in an undeclared region")
)

// Document is a parsed snapshot: the catalog it describes and the EU regions it
// declares.
type Document struct {
	// Catalog is the catalog the snapshot describes, built by catalog.New.
	Catalog catalog.Catalog
	// Regions are the regions the snapshot declares, in the order the document
	// lists them. Every offering of Catalog is available only in these regions.
	// The slice is the caller's own: each Parse returns a new one.
	Regions []string
}

// Parse decodes and validates data as a snapshot of schema version 1 and
// returns the catalog it describes, built by catalog.New, and the regions it
// declares. The package documentation lists every rule a document must meet.
//
// Every error is one line and wraps ErrDecode, ErrSchemaVersion,
// ErrUndeclaredRegion, or the error of catalog.New (so catalog.ErrInvalidCatalog
// and the taxonomy sentinels stay reachable). Values from data are shown through
// text.Quote; the text of the JSON decoder and time.Parse errors is never passed
// on. Errors name neither the input nor a target: the caller does.
func Parse(data []byte) (Document, error) {
	doc, err := decode(data)
	if err != nil {
		return Document{}, err
	}

	retrievedOn, err := parseDate(doc.retrievedOn)
	if err != nil {
		return Document{}, err
	}

	declared, err := declaredRegions(doc.regions)
	if err != nil {
		return Document{}, err
	}

	offerings := make([]catalog.Offering, len(doc.offerings))

	for i, o := range doc.offerings {
		for _, region := range o.regions {
			if _, ok := declared[region]; !ok {
				return Document{}, fmt.Errorf(
					"%w: offering %s is available in region %s, which the regions list does not declare",
					ErrUndeclaredRegion, text.Quote(o.name), text.Quote(region))
			}
		}

		offerings[i] = catalog.Offering{
			Name:         o.name,
			Categories:   o.categories,
			Capabilities: o.capabilities,
			Regions:      o.regions,
		}
	}

	snapshot := catalog.Snapshot{SourceURL: doc.sourceURL, RetrievedOn: retrievedOn}

	cat, err := catalog.New(doc.target, snapshot, offerings)
	if err != nil {
		return Document{}, err
	}

	return Document{Catalog: cat, Regions: doc.regions}, nil
}

// document is a snapshot of schema version 1 as the walk reads it, before its
// values are checked. Every field is required by the format; the walk refuses a
// document that leaves one out or gives it as null.
type document struct {
	target      string
	sourceURL   string
	retrievedOn string
	regions     []string
	offerings   []offering
}

// offering is one offering of a document. Its values are checked by catalog.New,
// not here.
type offering struct {
	name         string
	categories   []taxonomy.Category
	capabilities []taxonomy.Capability
	regions      []string
}

// The keys of schema version 1, spelt exactly as ADR 0007 records them.
const (
	keySchemaVersion = "schemaVersion"
	keyTarget        = "target"
	keySourceURL     = "sourceURL"
	keyRetrievedOn   = "retrievedOn"
	keyRegions       = "regions"
	keyOfferings     = "offerings"
	keyName          = "name"
	keyCategories    = "categories"
	keyCapabilities  = "capabilities"
)

// documentKeys and offeringKeys list the keys of the document object and of an
// offering object, every one of them required. They are never modified; a key's
// index is its bit in the mask walker.object keeps of the keys an object has.
var (
	documentKeys = [...]string{keySchemaVersion, keyTarget, keySourceURL, keyRetrievedOn, keyRegions, keyOfferings}
	offeringKeys = [...]string{keyName, keyCategories, keyCapabilities, keyRegions}
)

// decode reads data as a snapshot of schema version 1, in this order: at most
// MaxDocumentBytes; valid UTF-8; one JSON value, which is an object;
// schemaVersion 1, found before any other key or value is checked, so a
// document of another version is refused for its version alone; then, token by
// token, every key exact, known and not repeated, every field present, not null
// and of its JSON type, and no string that escapes a lone surrogate.
func decode(data []byte) (document, error) {
	if len(data) > MaxDocumentBytes {
		return document{}, fmt.Errorf("%w: the document is %d bytes, more than the limit of %d",
			ErrDecode, len(data), MaxDocumentBytes)
	}

	// The JSON decoder reads a byte that is not UTF-8, inside a string, as
	// U+FFFD and reports nothing, so the document's own bytes are checked first.
	// A UTF-16 byte order mark is not UTF-8; a UTF-8 one is, but is no JSON.
	if !utf8.Valid(data) {
		return document{}, fmt.Errorf("%w: the document is not valid UTF-8 at %s",
			ErrDecode, position(data, invalidUTF8(data)))
	}

	if bytes.HasPrefix(data, []byte(utf8BOM)) {
		return document{}, fmt.Errorf("%w: the document starts with a byte order mark; save it as UTF-8 without one",
			ErrDecode)
	}

	if !json.Valid(data) {
		return document{}, fmt.Errorf("%w: %s", ErrDecode, syntaxReason(data))
	}

	// json.Valid has refused a document that is only white space.
	if start := skipSpace(data, 0); data[start] != '{' {
		return document{}, fmt.Errorf("%w: the document is %s, not a JSON object", ErrDecode, kindOf(data[start]))
	}

	raw, misspelt, err := newWalker(data).findVersion()
	if err != nil {
		return document{}, err
	}

	if err := checkVersion(raw, misspelt); err != nil {
		return document{}, err
	}

	return newWalker(data).document()
}

// utf8BOM is the byte order mark U+FEFF encoded as UTF-8.
const utf8BOM = "\xef\xbb\xbf"

// invalidUTF8 returns the index of the first byte of data that is not valid
// UTF-8, or len(data) if there is none.
func invalidUTF8(data []byte) int {
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}

		i += size
	}

	return len(data)
}

// syntaxReason says why data, which json.Valid refused, is not one JSON value,
// in the package's own words.
//
// It reads data again with a json.Decoder, which tells a document that is empty
// or ends early from one with a wrong byte or with data after the value. Only the
// error path pays for it.
func syntaxReason(data []byte) string {
	dec := json.NewDecoder(bytes.NewReader(data))

	var value json.RawMessage

	err := dec.Decode(&value)

	var syntaxErr *json.SyntaxError

	switch {
	case errors.Is(err, io.EOF):
		return "the document is empty"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "the document ends in the middle of a JSON value"
	case errors.As(err, &syntaxErr):
		// The offset counts the character the decoder refused, every byte of
		// it: step back to its first byte.
		at := int(syntaxErr.Offset) - 1
		for at > 0 && !utf8.RuneStart(data[at]) {
			at--
		}

		return "malformed JSON at " + position(data, at)
	}

	// The first value is well formed, so what json.Valid refused comes after it.
	return "unexpected data after the catalog object at " + position(data, skipSpace(data, int(dec.InputOffset())))
}

// checkVersion checks the raw schemaVersion of a document: present, not null,
// and exactly the integer this package reads, spelt as such: 1.0 and "1" are not
// it. misspelt is a top-level key that differs from schemaVersion only in case,
// or "", and is named when the version is missing.
func checkVersion(raw []byte, misspelt string) error {
	switch string(raw) {
	case strconv.Itoa(schemaVersion):
		return nil
	case "", "null":
		if misspelt != "" {
			return fmt.Errorf("%w: schemaVersion is missing, want the integer %d (keys are exact: %s is not schemaVersion)",
				ErrSchemaVersion, schemaVersion, text.Quote(misspelt))
		}

		return fmt.Errorf("%w: schemaVersion is missing, want the integer %d", ErrSchemaVersion, schemaVersion)
	}

	return fmt.Errorf("%w: schemaVersion is %s, want the integer %d", ErrSchemaVersion, describeVersion(raw), schemaVersion)
}

// describeVersion names a schemaVersion value that is not the one this package
// reads: an integer as itself, a string by its value through text.Quote, and any
// other JSON value by its type. raw is one non-empty, well-formed JSON value.
func describeVersion(raw []byte) string {
	if n, err := strconv.Atoi(string(raw)); err == nil {
		return strconv.Itoa(n)
	}

	switch kind := kindOf(raw[0]); kind {
	case "a string":
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return "the string " + text.Quote(s)
		}

		return kind
	case "a number":
		return "the number " + text.Quote(string(raw))
	default:
		return kind
	}
}

// parseDate reads the retrieval date of a document: a YYYY-MM-DD calendar date,
// as UTC midnight.
func parseDate(raw string) (time.Time, error) {
	// A layout without a zone parses as UTC.
	date, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: retrievedOn %s is not a YYYY-MM-DD date", ErrDecode, text.Quote(raw))
	}

	return date, nil
}

// declaredRegions checks the regions a document declares, each non-blank, valid
// text and declared once, and returns them as a set, so an offering's regions
// are each checked in constant time however many regions are declared.
func declaredRegions(regions []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(regions))

	for i, region := range regions {
		if strings.TrimSpace(region) == "" {
			return nil, fmt.Errorf("%w: declared region %s (regions[%d]) is blank",
				ErrDecode, text.Quote(region), i)
		}

		if err := text.Validate(region); err != nil {
			return nil, fmt.Errorf("%w: declared region %s (regions[%d]): %w",
				ErrDecode, text.Quote(region), i, err)
		}

		if _, dup := set[region]; dup {
			return nil, fmt.Errorf("%w: region %s is declared more than once (again at regions[%d])",
				ErrDecode, text.Quote(region), i)
		}

		set[region] = struct{}{}
	}

	return set, nil
}

// kindOf names, with its article, the JSON type of the value whose first byte
// is b.
func kindOf(b byte) string {
	switch b {
	case '{':
		return "an object"
	case '[':
		return "an array"
	case '"':
		return "a string"
	case 't', 'f':
		return "a boolean"
	case 'n':
		return "null"
	default:
		return "a number"
	}
}

// position says where byte i of data is, as a reader of the document looks it
// up: "line L, column C", both counted from 1, the line by line feeds and the
// column in characters, so a tab or a multi-byte character counts once. data up
// to i is valid UTF-8 wherever position is used.
func position(data []byte, i int) string {
	before := data[:min(max(i, 0), len(data))]
	line := bytes.Count(before, []byte{'\n'}) + 1
	column := utf8.RuneCount(before[bytes.LastIndexByte(before, '\n')+1:]) + 1

	return fmt.Sprintf("line %d, column %d", line, column)
}

// skipSpace returns the index of the first byte of data at or after i that is
// not JSON white space, or len(data).
func skipSpace(data []byte, i int) int {
	for i < len(data) && isSpace(data[i]) {
		i++
	}

	return min(i, len(data))
}

// isSpace reports whether b is JSON white space (RFC 8259, section 2).
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
