package inventoryfile

import (
	"context"
	"errors"
	"fmt"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/internal/jsondoc"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
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

	return inventory.New(doc.source, doc.resources)
}

// document is an inventory file of schema version 1, as ADR 0006 records it and
// the walk reads it, before inventory.New checks its values. Its schemaVersion
// is checked as the document is read and not kept.
type document struct {
	source    string
	resources []inventory.Resource
}

// The keys of schema version 1, spelt exactly as ADR 0006 records them.
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
// resource object. Only resources is required of the walk: the version is
// checked before it, and an absent source, id or category reaches inventory.New
// as an empty value, which the domain refuses with its own sentinel. They are
// never modified.
var (
	documentKeys = []jsondoc.Key{
		{Name: keySchemaVersion},
		{Name: keySource},
		{Name: keyResources, Required: true},
	}
	resourceKeys = []jsondoc.Key{
		{Name: keyID},
		{Name: keyRawType},
		{Name: keyCategory},
		{Name: keyRegion},
		{Name: keyCapabilities},
	}
)

// decode decodes data as an inventory document of schema version 1 through the
// shared strict decoder (jsondoc), in this order: at most MaxFileSize; UTF-8
// text without a byte order mark; one JSON value, which is an object;
// schemaVersion 1 (jsondoc.Document.Version), found before any key or type is
// checked, so a file of another version is refused for its version, never for a
// field this version does not know; then, token by token (walk), every key
// exact, known and not repeated, every value of the right JSON type, no string
// that escapes a lone surrogate, a resources list, at most MaxResources
// resources and at most MaxCapabilities capabilities in each, each counted
// before it is read.
func decode(data []byte) (document, error) {
	doc, err := jsondoc.Open(data, MaxFileSize, ErrDecode)
	if err != nil {
		return document{}, err
	}

	if err := doc.Version(keySchemaVersion, schemaVersion, ErrSchemaVersion); err != nil {
		return document{}, err
	}

	return walk(doc.Walker())
}
