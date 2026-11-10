package inventoryfile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"

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

// MaxCapabilities is the most capabilities one resource may list. The taxonomy
// vocabulary is far smaller, so a longer list is a malformed file, refused
// while the document is read, before its capabilities are decoded.
const MaxCapabilities = 32

// schemaVersion is the only version of the inventory file format this package
// reads (ADR 0006).
const schemaVersion = 1

var (
	// ErrRead is returned when the inventory file cannot be opened or read.
	ErrRead = errors.New("cannot read")
	// ErrTooLarge is returned for an inventory file larger than MaxFileSize.
	ErrTooLarge = errors.New("file too large")
	// ErrDecode is returned for a file that is not a well-formed inventory
	// document: not UTF-8 JSON, more than one JSON value, a string that escapes
	// a lone surrogate, a field of the wrong type, a key that is unknown,
	// misspelt or repeated, a required field missing, or a resource with more
	// than MaxCapabilities capabilities.
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
// A cancelled ctx is reported before the file is touched, and cancelling ctx
// while the file is opened or read ends the call at once, even when the open or
// the read blocks (a FIFO with no writer, a stalled network file system). Every
// error is one line that names the file through text.QuotePath, which keeps the
// base name of a long path, and wraps one of the package's sentinels, a domain
// sentinel (such as taxonomy.ErrUnknownCategory) for a value the domain refuses,
// or ctx.Err(). Values from the file are shown through text.Quote; the text of
// the OS and JSON decoder errors is never passed on.
func (s *Source) Inventory(ctx context.Context) (inventory.Inventory, error) {
	inv, err := s.inventory(ctx)
	if err != nil {
		return inventory.Inventory{}, fmt.Errorf("inventory file %s: %w", text.QuotePath(s.path), err)
	}

	return inv, nil
}

// inventory does the work of Inventory; its errors do not name the file.
func (s *Source) inventory(ctx context.Context) (inventory.Inventory, error) {
	if err := ctx.Err(); err != nil {
		return inventory.Inventory{}, err
	}

	data, err := s.read(ctx)
	if err != nil {
		return inventory.Inventory{}, err
	}

	doc, err := decode(data)
	if err != nil {
		return inventory.Inventory{}, err
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

// document is an inventory file of schema version 1, as ADR 0006 records it.
type document struct {
	// SchemaVersion is checked before the document is decoded (findVersion,
	// walker.document); it is declared so the struct records the whole format.
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

// The keys of schema version 1, spelt exactly as ADR 0006 records them. The
// struct tags of document and resource repeat them.
const (
	keySchemaVersion = "schemaVersion"
	keySource        = "source"
	keyResources     = "resources"
	keyID            = "id"
	keyRawType       = "rawType"
	keyCategory      = "category"
	keyRegion        = "region"
	keyCapabilities  = "capabilities"
)

// documentKeys and resourceKeys list the keys of the document object and of a
// resource object. They are never modified; a key's index is its bit in the
// mask walker.key keeps of the keys an object has.
var (
	documentKeys = [...]string{keySchemaVersion, keySource, keyResources}
	resourceKeys = [...]string{keyID, keyRawType, keyCategory, keyRegion, keyCapabilities}
)

// decode decodes data as an inventory document of schema version 1, in this
// order: UTF-8 text without a byte order mark; one JSON value, which is an
// object; schemaVersion 1, found before any key or type is checked, so a file
// of another version is refused for its version, never for a field this version
// does not know; then, token by token (walker), every key exact, known and not
// repeated, every value of the right JSON type, no string that escapes a lone
// surrogate, at most MaxResources resources and at most MaxCapabilities
// capabilities in each; and only then the decoded document, with its required
// fields present.
func decode(data []byte) (document, error) {
	if err := checkEncoding(data); err != nil {
		return document{}, err
	}

	// json.Unmarshal checks the syntax of the whole document before it decodes
	// anything, and needs no copy of data to do so. Into a pointer to an empty
	// struct it decodes nothing, but tells an object from null or another type.
	var object *struct{}

	err := json.Unmarshal(data, &object)

	var typeErr *json.UnmarshalTypeError

	switch {
	case errors.As(err, &typeErr) && typeErr.Field == "", err == nil && object == nil:
		start := skipSpace(data, 0)

		return document{}, fmt.Errorf("%w: the document is %s, not a JSON object", ErrDecode, kindOf(data[start]))
	case err != nil:
		return document{}, fmt.Errorf("%w: %s", ErrDecode, syntaxReason(data))
	}

	raw, misspelt, err := newWalker(data).findVersion()
	if err != nil {
		return document{}, err
	}

	if err := checkVersion(raw, misspelt); err != nil {
		return document{}, err
	}

	if err := newWalker(data).document(); err != nil {
		return document{}, err
	}

	// The walk has checked every key and type, so decoding cannot fail; the
	// error is still checked rather than trusted.
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return document{}, fmt.Errorf("%w: the JSON document cannot be decoded", ErrDecode)
	}

	if doc.Resources == nil {
		return document{}, fmt.Errorf("%w: resources is missing or null, want an array", ErrDecode)
	}

	return doc, nil
}

// checkEncoding refuses data that is not UTF-8 text without a byte order mark.
// encoding/json would replace an invalid byte with U+FFFD and read on, so the
// values decoded would differ from the bytes in the file.
func checkEncoding(data []byte) error {
	for _, bom := range [][]byte{{0xEF, 0xBB, 0xBF}, {0xFF, 0xFE}, {0xFE, 0xFF}} {
		if bytes.HasPrefix(data, bom) {
			return fmt.Errorf("%w: the file starts with a byte order mark; save the file as UTF-8 without one", ErrDecode)
		}
	}

	if !utf8.Valid(data) {
		return fmt.Errorf("%w: the file is not valid UTF-8 at %s", ErrDecode, position(data, invalidUTF8(data)))
	}

	return nil
}

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

// syntaxReason says why data, which json.Unmarshal refused as malformed, is not
// one JSON value, in the package's own words.
//
// It reads data again with a json.Decoder, which tells a file that is empty or
// ends early from one with a wrong byte or with data after the value. Only the
// error path pays for the decoder's copy of data.
func syntaxReason(data []byte) string {
	dec := json.NewDecoder(bytes.NewReader(data))

	var value struct{}

	err := dec.Decode(&value)

	var syntaxErr *json.SyntaxError

	switch {
	case errors.Is(err, io.EOF):
		return "the file is empty"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "the file ends in the middle of the JSON document"
	case errors.As(err, &syntaxErr):
		// The offset counts the byte the decoder refused.
		return "malformed JSON at " + position(data, int(syntaxErr.Offset)-1)
	}

	// The first value is well formed (its type does not matter here), so what
	// json.Unmarshal refused comes after it.
	return "unexpected data after the first JSON value at " + position(data, skipSpace(data, int(dec.InputOffset())))
}

// checkVersion checks the raw schemaVersion of a file: present, not null, and
// exactly the integer this package reads, spelt as such: 1.0 and "1" are not it.
// misspelt is a top-level key that differs from schemaVersion only in case, or
// "", and is named when the version is missing.
func checkVersion(raw []byte, misspelt string) error {
	switch string(raw) {
	case strconv.Itoa(schemaVersion):
		return nil
	case "", "null":
		if misspelt != "" {
			return fmt.Errorf("%w: schemaVersion is missing, want %d (keys are exact: %s is not schemaVersion)",
				ErrSchemaVersion, schemaVersion, text.Quote(misspelt))
		}

		return fmt.Errorf("%w: schemaVersion is missing, want %d", ErrSchemaVersion, schemaVersion)
	}

	if n, err := strconv.Atoi(string(raw)); err == nil {
		return fmt.Errorf("%w: schemaVersion is %d, want %d", ErrSchemaVersion, n, schemaVersion)
	}

	seen := kindOf(raw[0])
	if seen == "a number" {
		seen = "the number " + text.Quote(string(raw))
	}

	return fmt.Errorf("%w: schemaVersion is %s, want the integer %d", ErrSchemaVersion, seen, schemaVersion)
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

// position says where byte i of data is, as a reader of the file looks it up:
// "line L, column C", both counted from 1, the line by line feeds and the
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
