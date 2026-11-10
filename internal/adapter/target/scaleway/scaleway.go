package scaleway

import (
	"bytes"
	"context"
	_ "embed" // the catalog snapshot is embedded with //go:embed
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

// embeddedName is the name of the embedded catalog file, as errors show it.
const embeddedName = "catalog.json"

// embedded is the dated catalog snapshot the adapter serves. SOURCES.md records
// where its data comes from.
//
//go:embed catalog.json
var embedded []byte

// schemaVersion is the only version of the catalog document format this
// package reads.
const schemaVersion = 1

// MaxDocumentBytes is the size of the largest catalog document Parse reads, in
// bytes. A larger one is refused before any of it is decoded. The embedded
// snapshot is a few kilobytes; the limit leaves room to grow while keeping a
// document that is not a catalog from costing more than a moment to refuse.
const MaxDocumentBytes = 1 << 20

var (
	// ErrDecode is returned for data that is not a well-formed catalog document:
	// larger than MaxDocumentBytes, not valid UTF-8, not JSON, not a JSON
	// object, more than one JSON value, a field of the wrong type, an unknown
	// field, a missing or null regions or offerings list, a declared region that
	// is blank, not valid text (text.Validate) or declared twice, or a retrieval
	// date that is missing or not a YYYY-MM-DD date.
	ErrDecode = errors.New("not a valid catalog document")
	// ErrSchemaVersion is returned for a document whose schemaVersion is missing
	// or is not a version this package reads.
	ErrSchemaVersion = errors.New("unsupported schema version")
	// ErrUndeclaredRegion is returned for an offering available in a region the
	// document's top-level regions list does not declare.
	ErrUndeclaredRegion = errors.New("offering in an undeclared region")
)

// Source serves the Scaleway catalog from the embedded snapshot. It implements
// the analysis.CatalogSource port.
type Source struct{}

// New returns a Source that serves the embedded Scaleway catalog snapshot.
func New() *Source {
	return &Source{}
}

// Catalog returns the catalog the embedded snapshot describes, built by
// catalog.New. Every call returns the same catalog.
//
// A cancelled ctx is reported before the snapshot is read. Every error is one
// line that names the embedded file and wraps ctx.Err() or an error of Parse.
func (s *Source) Catalog(ctx context.Context) (catalog.Catalog, error) {
	if err := ctx.Err(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("embedded Scaleway catalog %s: %w", text.Quote(embeddedName), err)
	}

	cat, _, err := parse(embedded)
	if err != nil {
		return catalog.Catalog{}, fmt.Errorf("embedded Scaleway catalog %s: %w", text.Quote(embeddedName), err)
	}

	return cat, nil
}

// Regions returns the EU regions the embedded snapshot declares, in the order
// the file lists them. Every offering of the catalog is available only in these
// regions. The result is the caller's own copy.
//
// It returns nil if the embedded snapshot does not parse, which the package's
// tests rule out; Catalog reports why.
func Regions() []string {
	_, regions, err := parse(embedded)
	if err != nil {
		return nil
	}

	return regions
}

// Parse decodes and validates data as a catalog document and returns the
// catalog it describes, built by catalog.New.
//
// The document is at most MaxDocumentBytes long and valid UTF-8: one JSON
// object, with nothing after it but white space, and no field the format does
// not define. Its
// schemaVersion must be the integer 1 and its retrievedOn a YYYY-MM-DD date
// (read as UTC midnight). Its regions and offerings lists must be present and
// not null. Every declared region must be non-blank, valid text
// (text.Validate) and declared once, and every offering region must be one of
// them.
//
// Every error is one line and wraps ErrDecode, ErrSchemaVersion,
// ErrUndeclaredRegion, or the error of catalog.New (so catalog.ErrInvalidCatalog
// and the taxonomy sentinels stay reachable). Values from data are shown
// through text.Quote; the text of the JSON decoder and time.Parse errors is
// never passed on. Errors do not name the input: the caller does.
func Parse(data []byte) (catalog.Catalog, error) {
	cat, _, err := parse(data)

	return cat, err
}

// parse does the work of Parse and also returns the regions the document
// declares.
func parse(data []byte) (catalog.Catalog, []string, error) {
	if len(data) > MaxDocumentBytes {
		return catalog.Catalog{}, nil, fmt.Errorf("%w: the document is %d bytes, more than the limit of %d",
			ErrDecode, len(data), MaxDocumentBytes)
	}

	// The JSON decoder reads a byte that is not UTF-8, inside a string, as
	// U+FFFD and reports nothing, so the document's own bytes are checked first.
	if !utf8.Valid(data) {
		return catalog.Catalog{}, nil, fmt.Errorf("%w: the document is not valid UTF-8 at byte offset %d",
			ErrDecode, invalidUTF8Offset(data))
	}

	doc, err := decode(data)
	if err != nil {
		return catalog.Catalog{}, nil, err
	}

	retrievedOn, err := parseDate(doc.RetrievedOn)
	if err != nil {
		return catalog.Catalog{}, nil, err
	}

	declared, err := declaredRegions(*doc.Regions)
	if err != nil {
		return catalog.Catalog{}, nil, err
	}

	offerings := make([]catalog.Offering, len(*doc.Offerings))

	for i, o := range *doc.Offerings {
		for _, region := range o.Regions {
			if _, ok := declared[region]; !ok {
				return catalog.Catalog{}, nil, fmt.Errorf(
					"%w: offering %s is available in region %s, which the regions list does not declare",
					ErrUndeclaredRegion, text.Quote(o.Name), text.Quote(region))
			}
		}

		offerings[i] = catalog.Offering{
			Name:         o.Name,
			Categories:   o.Categories,
			Capabilities: o.Capabilities,
			Regions:      o.Regions,
		}
	}

	snapshot := catalog.Snapshot{SourceURL: doc.SourceURL, RetrievedOn: retrievedOn}

	cat, err := catalog.New(doc.Target, snapshot, offerings)
	if err != nil {
		return catalog.Catalog{}, nil, err
	}

	return cat, *doc.Regions, nil
}

// invalidUTF8Offset returns the byte offset of the first byte of data that does
// not start a valid UTF-8 sequence, or -1 if data is valid UTF-8.
func invalidUTF8Offset(data []byte) int {
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}

		i += size
	}

	return -1
}

// declaredRegions checks the regions a document declares, each non-blank,
// valid text and declared once, and returns them as a set, so an offering's
// regions are each checked in constant time.
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

// parseDate reads the retrieval date of a document: a YYYY-MM-DD calendar date,
// as UTC midnight.
func parseDate(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf("%w: field retrievedOn is missing", ErrDecode)
	}

	// A layout without a zone parses as UTC.
	date, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: retrievedOn %s is not a YYYY-MM-DD date", ErrDecode, text.Quote(raw))
	}

	return date, nil
}

// envelope is what Parse decodes before it knows the schema version: only the
// version, so a document of another version is refused for its version, never
// for a field this version does not define.
type envelope struct {
	SchemaVersion json.RawMessage `json:"schemaVersion"`
}

// document is a catalog document of schema version 1.
type document struct {
	// SchemaVersion is checked through the envelope; it is declared here so
	// strict decoding accepts it.
	SchemaVersion json.RawMessage `json:"schemaVersion"`
	Target        string          `json:"target"`
	SourceURL     string          `json:"sourceURL"`
	RetrievedOn   string          `json:"retrievedOn"`
	// Regions and Offerings are pointers so a list the document leaves out, or
	// gives as null, is told apart from an empty one: decode refuses both.
	Regions   *[]string   `json:"regions"`
	Offerings *[]offering `json:"offerings"`
}

// offering is one offering of a document. Its values are checked by
// catalog.New, not here.
type offering struct {
	Name         string                `json:"name"`
	Categories   []taxonomy.Category   `json:"categories"`
	Capabilities []taxonomy.Capability `json:"capabilities"`
	Regions      []string              `json:"regions"`
}

// decode decodes data as a catalog document of schema version 1: one JSON
// object, nothing after it, every field one the format defines, and the
// regions and offerings lists present and not null.
func decode(data []byte) (document, error) {
	// A pointer, so a top-level null, which leaves it nil, is told apart from
	// an object without a schemaVersion: null is no catalog document at all.
	var env *envelope
	if err := decodeOne(data, &env, false); err != nil {
		return document{}, err
	}

	if env == nil {
		return document{}, fmt.Errorf("%w: the document is null, not a JSON object", ErrDecode)
	}

	if err := checkVersion(env.SchemaVersion); err != nil {
		return document{}, err
	}

	var doc document
	if err := decodeOne(data, &doc, true); err != nil {
		return document{}, err
	}

	if doc.Regions == nil {
		return document{}, fmt.Errorf("%w: field regions is missing or null", ErrDecode)
	}

	if doc.Offerings == nil {
		return document{}, fmt.Errorf("%w: field offerings is missing or null", ErrDecode)
	}

	return doc, nil
}

// decodeOne decodes the single JSON value in data into v and refuses anything
// after it but white space. With strict, a field v does not declare is an
// error.
func decodeOne(data []byte, v any, strict bool) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if strict {
		dec.DisallowUnknownFields()
	}

	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %s", ErrDecode, decodeReason(err))
	}

	end := dec.InputOffset()

	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: unexpected data after the catalog object, which ends at byte offset %d", ErrDecode, end)
	}

	return nil
}

// unknownFieldPrefix starts the error encoding/json returns for a field the
// target type does not declare, followed by the field name as a Go string
// literal.
const unknownFieldPrefix = "json: unknown field "

// decodeReason describes a JSON decoding error in the package's own words: what
// is wrong and where, never the decoder's text, and any value from the document
// through text.Quote.
func decodeReason(err error) string {
	var (
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
	)

	switch {
	case errors.Is(err, io.EOF):
		return "the document is empty"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "the document ends in the middle of a JSON value"
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("malformed JSON at byte offset %d", syntaxErr.Offset)
	case errors.As(err, &typeErr):
		if typeErr.Field == "" {
			return fmt.Sprintf("the document is not a JSON object (byte offset %d)", typeErr.Offset)
		}

		return fmt.Sprintf("field %s at byte offset %d has the wrong JSON type", text.Quote(typeErr.Field), typeErr.Offset)
	}

	if quoted, ok := strings.CutPrefix(err.Error(), unknownFieldPrefix); ok {
		if field, uerr := strconv.Unquote(quoted); uerr == nil {
			return "unknown field " + text.Quote(field)
		}

		return "unknown field"
	}

	return "the JSON document cannot be decoded"
}

// checkVersion checks the raw schemaVersion of a document: present, not null,
// and exactly the integer this package reads.
func checkVersion(raw json.RawMessage) error {
	switch string(raw) {
	case strconv.Itoa(schemaVersion):
		return nil
	case "", "null":
		return fmt.Errorf("%w: schemaVersion is missing, want the integer %d", ErrSchemaVersion, schemaVersion)
	}

	return fmt.Errorf("%w: schemaVersion is %s, want the integer %d", ErrSchemaVersion, describeVersion(raw), schemaVersion)
}

// describeVersion names a schemaVersion value that is not the one this package
// reads: an integer as itself, a string by its value through text.Quote, and
// any other JSON value by its type. raw is one non-empty, well-formed JSON
// value, as the decoder captured it.
func describeVersion(raw json.RawMessage) string {
	if n, err := strconv.Atoi(string(raw)); err == nil {
		return strconv.Itoa(n)
	}

	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return "the string " + text.Quote(s)
		}

		return "a string"
	case '{':
		return "a JSON object"
	case '[':
		return "a JSON array"
	case 't', 'f':
		return "a boolean"
	}

	return "the number " + text.Quote(string(raw))
}
