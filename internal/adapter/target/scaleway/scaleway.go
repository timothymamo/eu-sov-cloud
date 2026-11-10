package scaleway

import (
	"bytes"
	"context"
	_ "embed" // the catalog snapshot is embedded with //go:embed
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

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

var (
	// ErrDecode is returned for data that is not a well-formed catalog document:
	// not JSON, more than one JSON value, a field of the wrong type, an unknown
	// field, or a retrieval date that is missing or not a YYYY-MM-DD date.
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
// The document is one JSON object, with nothing after it but white space, and
// no field the format does not define. Its schemaVersion must be 1, its
// retrievedOn a YYYY-MM-DD date (read as UTC midnight), and every offering
// region must be in the top-level regions list.
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
	doc, err := decode(data)
	if err != nil {
		return catalog.Catalog{}, nil, err
	}

	retrievedOn, err := parseDate(doc.RetrievedOn)
	if err != nil {
		return catalog.Catalog{}, nil, err
	}

	offerings := make([]catalog.Offering, len(doc.Offerings))

	for i, o := range doc.Offerings {
		for _, region := range o.Regions {
			if !slices.Contains(doc.Regions, region) {
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

	return cat, doc.Regions, nil
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
	Regions       []string        `json:"regions"`
	Offerings     []offering      `json:"offerings"`
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
// object, nothing after it, and every field one the format defines.
func decode(data []byte) (document, error) {
	var env envelope
	if err := decodeOne(data, &env, false); err != nil {
		return document{}, err
	}

	if err := checkVersion(env.SchemaVersion); err != nil {
		return document{}, err
	}

	var doc document
	if err := decodeOne(data, &doc, true); err != nil {
		return document{}, err
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
		return fmt.Errorf("%w: schemaVersion is missing, want %d", ErrSchemaVersion, schemaVersion)
	}

	seen := text.Quote(string(raw))
	if n, err := strconv.Atoi(string(raw)); err == nil {
		seen = strconv.Itoa(n)
	}

	return fmt.Errorf("%w: schemaVersion is %s, want %d", ErrSchemaVersion, seen, schemaVersion)
}
