package inventoryfile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// MaxFileSize is the largest inventory file Inventory reads, in bytes. A larger
// file is refused before it is decoded.
const MaxFileSize = 10 << 20

// MaxResources is the most resources an inventory file may hold. Each resource
// is mapped against every candidate offering, so the count is capped as well as
// the size.
const MaxResources = 50000

// schemaVersion is the only version of the inventory file format this package
// reads (ADR 0006).
const schemaVersion = 1

var (
	// ErrRead is returned when the inventory file cannot be opened or read.
	ErrRead = errors.New("cannot read")
	// ErrTooLarge is returned for an inventory file larger than MaxFileSize.
	ErrTooLarge = errors.New("file too large")
	// ErrDecode is returned for a file that is not a well-formed inventory
	// document: not JSON, more than one JSON value, a field of the wrong type, an
	// unknown field, or a required field missing.
	ErrDecode = errors.New("not a valid inventory document")
	// ErrSchemaVersion is returned for a file whose schemaVersion is missing or
	// is not a version this package reads.
	ErrSchemaVersion = errors.New("unsupported schema version")
	// ErrTooManyResources is returned for a file that holds more than
	// MaxResources resources.
	ErrTooManyResources = errors.New("too many resources")
)

// Source reads the inventory of a source from a JSON inventory file. It
// implements the analysis.InventorySource port.
type Source struct {
	path string
}

// New returns a Source that reads the inventory file at path. The file is read
// on every call to Inventory, never by New.
func New(path string) *Source {
	return &Source{path: path}
}

// Inventory reads, decodes and validates the inventory file and returns the
// inventory it describes, built by inventory.New.
//
// A cancelled ctx is reported before the file is touched. Every error is one
// line that names the file through text.Quote and wraps one of the package's
// sentinels, a domain sentinel (such as taxonomy.ErrUnknownCategory) for a value
// the domain refuses, or ctx.Err(). Values from the file are shown through
// text.Quote; the text of the OS and JSON decoder errors is never passed on.
func (s *Source) Inventory(ctx context.Context) (inventory.Inventory, error) {
	inv, err := s.inventory(ctx)
	if err != nil {
		return inventory.Inventory{}, fmt.Errorf("inventory file %s: %w", text.Quote(s.path), err)
	}

	return inv, nil
}

// inventory does the work of Inventory; its errors do not name the file.
func (s *Source) inventory(ctx context.Context) (inventory.Inventory, error) {
	if err := ctx.Err(); err != nil {
		return inventory.Inventory{}, err
	}

	data, err := s.read()
	if err != nil {
		return inventory.Inventory{}, err
	}

	doc, err := decode(data)
	if err != nil {
		return inventory.Inventory{}, err
	}

	if n := len(*doc.Resources); n > MaxResources {
		return inventory.Inventory{}, fmt.Errorf("%w: %d, at most %d", ErrTooManyResources, n, MaxResources)
	}

	resources := make([]inventory.Resource, len(*doc.Resources))
	for i, r := range *doc.Resources {
		resources[i] = inventory.Resource{
			ID:           r.ID,
			RawType:      r.RawType,
			Category:     r.Category,
			Region:       r.Region,
			Capabilities: r.Capabilities,
		}
	}

	return inventory.New(doc.Source, resources)
}

// read returns the contents of the file, refusing one larger than MaxFileSize
// without reading more than one byte past the cap.
func (s *Source) read() ([]byte, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrRead, osReason(err))
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrRead, osReason(err))
	}

	if len(data) > MaxFileSize {
		return nil, fmt.Errorf("%w: more than %d bytes", ErrTooLarge, MaxFileSize)
	}

	return data, nil
}

// osReason describes why opening or reading the file failed, in the package's
// own words. It never returns the text of an *fs.PathError, which repeats the
// path unquoted: only the OS's reason inside it, quoted.
func osReason(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "no such file"
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	}

	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return text.Quote(pathErr.Err.Error())
	}

	return "unexpected I/O error"
}

// envelope is what Inventory decodes before it knows the schema version: only
// the version, so a file of another version is refused for its version, never
// for a field this version does not know.
type envelope struct {
	SchemaVersion json.RawMessage `json:"schemaVersion"`
}

// document is an inventory file of schema version 1, as ADR 0006 records it.
type document struct {
	// SchemaVersion is checked through the envelope; it is declared here so
	// strict decoding accepts it.
	SchemaVersion json.RawMessage `json:"schemaVersion"`
	Source        string          `json:"source"`
	// Resources is required: it is nil when the list is missing or null, and
	// points to an empty slice for an empty list.
	Resources *[]resource `json:"resources"`
}

// resource is one resource of a document. Its values are checked by
// inventory.New, not here.
type resource struct {
	ID           string                `json:"id"`
	RawType      string                `json:"rawType"`
	Category     taxonomy.Category     `json:"category"`
	Region       string                `json:"region"`
	Capabilities []taxonomy.Capability `json:"capabilities"`
}

// decode decodes data as an inventory document of schema version 1: one JSON
// object, nothing after it, every field known and of the right type, and the
// required fields present.
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

	if doc.Resources == nil {
		return document{}, fmt.Errorf("%w: field resources is missing", ErrDecode)
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

	var rest json.RawMessage
	if err := dec.Decode(&rest); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: unexpected data after the inventory object, which ends at byte offset %d", ErrDecode, end)
	}

	return nil
}

// unknownFieldPrefix starts the error encoding/json returns for a field the
// target does not declare, followed by the field name as a Go string literal.
const unknownFieldPrefix = "json: unknown field "

// decodeReason describes a JSON decoding error in the package's own words: what
// is wrong and where, never the decoder's text, and any value from the file
// through text.Quote.
func decodeReason(err error) string {
	var (
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
	)

	switch {
	case errors.Is(err, io.EOF):
		return "the file is empty"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "the file ends in the middle of the JSON document"
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("malformed JSON at byte offset %d", syntaxErr.Offset)
	case errors.As(err, &typeErr):
		if typeErr.Field == "" {
			return fmt.Sprintf("the document is not a JSON object (byte offset %d)", typeErr.Offset)
		}

		return fmt.Sprintf("field %s at byte offset %d is not %s",
			text.Quote(typeErr.Field), typeErr.Offset, jsonType(typeErr.Type))
	}

	if quoted, ok := strings.CutPrefix(err.Error(), unknownFieldPrefix); ok {
		if field, uerr := strconv.Unquote(quoted); uerr == nil {
			return "unknown field " + text.Quote(field)
		}

		return "unknown field"
	}

	return "the JSON document cannot be decoded"
}

// jsonType names the JSON type that decodes into t, with its article.
func jsonType(t reflect.Type) string {
	if t == nil {
		return "of the expected type"
	}

	switch t.Kind() {
	case reflect.Pointer:
		return jsonType(t.Elem())
	case reflect.String:
		return "a string"
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.Struct, reflect.Map:
		return "an object"
	case reflect.Bool:
		return "a boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "an integer"
	case reflect.Float32, reflect.Float64:
		return "a number"
	default:
		return "of the expected type"
	}
}

// checkVersion checks the raw schemaVersion of a file: present, not null, and
// exactly the integer this package reads.
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
