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
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
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
// error is one line that names the file through text.Quote and wraps one of the
// package's sentinels, a domain sentinel (such as taxonomy.ErrUnknownCategory)
// for a value the domain refuses, or ctx.Err(). Values from the file are shown
// through text.Quote; the text of the OS and JSON decoder errors is never passed
// on.
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

// readResult is what the goroutine of read sends back.
type readResult struct {
	data []byte
	err  error
}

// read returns the contents of the file, refusing one larger than MaxFileSize.
//
// The file is opened and read on a goroutine of its own, so that a cancelled
// ctx ends the call even while the open or a read blocks. Cancelling closes the
// file once it is open, which ends a read blocked on a pipe or FIFO. An open
// that blocks (a FIFO with no writer) cannot be interrupted portably, and
// neither can a read Go cannot poll (a regular file on a stalled network file
// system): the goroutine then waits until the open or the read returns, or
// until the process exits, which for the CLI is the end of the run. The call
// returns all the same. The file is read once per call, so a pipe serves a
// single read.
func (s *Source) read(ctx context.Context) ([]byte, error) {
	done := make(chan readResult, 1)

	go func() {
		f, err := os.Open(s.path)
		if err != nil {
			done <- readResult{err: fmt.Errorf("%w: %s", ErrRead, osReason(err))}

			return
		}
		defer f.Close()

		// Closing the file ends a read blocked on it; on the way out, stop
		// keeps a later cancellation from closing it again.
		stop := context.AfterFunc(ctx, func() { _ = f.Close() })
		defer stop()

		data, err := readCapped(f)
		done <- readResult{data: data, err: err}
	}()

	select {
	case r := <-done:
		if r.err != nil && ctx.Err() != nil {
			// The read failed because cancelling closed the file.
			return nil, ctx.Err()
		}

		return r.data, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// readCapped reads r to its end, refusing more than MaxFileSize bytes without
// reading more than one byte past the cap.
func readCapped(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxFileSize+1))
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

// walker reads data, a well-formed JSON object, token by token before it is
// decoded. A json.Decoder reading tokens holds one token at a time, however
// large the document or its values.
//
// encoding/json decodes keys ignoring case (and Unicode folding maps the long s
// to s), keeps the last of a repeated key, decodes an escaped lone surrogate to
// U+FFFD, and decodes every resource before any of them can be counted; walker
// reads the document as it is written, character for character once JSON
// escapes are decoded, and its errors say where in the file the problem is, as
// a line and a column.
type walker struct {
	data []byte
	dec  *json.Decoder
}

// newWalker returns a walker at the start of data.
func newWalker(data []byte) *walker {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	return &walker{data: data, dec: dec}
}

// findVersion returns the raw value of the document's schemaVersion key, spelt
// exactly so, the first one if the key is repeated, or nil if there is none.
// misspelt is the first top-level key before it that differs from schemaVersion
// only in case. It reads the top-level keys only, stepping over every other
// value, and stops at the version, which a writer puts first.
func (w *walker) findVersion() (raw []byte, misspelt string, err error) {
	if _, _, err := w.next(); err != nil {
		return nil, "", err
	}

	for w.dec.More() {
		tok, _, err := w.next()
		if err != nil {
			return nil, "", err
		}

		key, _ := tok.(string)
		if key != keySchemaVersion && misspelt == "" && strings.EqualFold(key, keySchemaVersion) {
			misspelt = key
		}

		at, err := w.skip()
		if err != nil {
			return nil, "", err
		}

		if key == keySchemaVersion {
			return w.data[at:w.dec.InputOffset()], misspelt, nil
		}
	}

	return nil, misspelt, nil
}

// skip steps over the next value, object and array included, and returns the
// index in data of its first byte.
func (w *walker) skip() (int, error) {
	depth := 0

	tok, at, err := w.next()

	for ; err == nil; tok, _, err = w.next() {
		switch tok {
		case json.Delim('{'), json.Delim('['):
			depth++
		case json.Delim('}'), json.Delim(']'):
			depth--
		}

		if depth == 0 {
			return at, nil
		}
	}

	return at, err
}

// next returns the next token and the index in data of its first byte.
func (w *walker) next() (json.Token, int, error) {
	at := skipSeparators(w.data, int(w.dec.InputOffset()))

	tok, err := w.dec.Token()
	if err != nil {
		// json.Unmarshal has checked the syntax already, so the decoder fails
		// here only if the walker has lost its place.
		return nil, at, fmt.Errorf("%w: the JSON document cannot be read at %s", ErrDecode, position(w.data, at))
	}

	return tok, at, nil
}

// document walks the whole document, a JSON object whose exact schemaVersion
// key findVersion found to be 1. It refuses a key that is not spelt exactly as
// the format defines it, a key an object already has, a value of the wrong JSON
// type, a string that escapes a lone surrogate, more than MaxResources
// resources and more than MaxCapabilities capabilities in a resource, counted
// before a single resource is decoded.
func (w *walker) document() error {
	// json.Unmarshal decoded the envelope from this object: the first token is
	// its opening brace.
	if _, _, err := w.next(); err != nil {
		return err
	}

	var seen uint

	for w.dec.More() {
		key, err := w.key(documentKeys[:], &seen, "")
		if err != nil {
			return err
		}

		switch key {
		case keySchemaVersion:
			err = w.version()
		case keyResources:
			err = w.resources()
		default:
			err = w.text("", key)
		}

		if err != nil {
			return err
		}
	}

	_, _, err := w.next()

	return err
}

// key reads the next key of an object, at path parent ("" for the document),
// whose keys are known, and refuses a key that escapes a lone surrogate, that
// is not character for character one of them once JSON escapes are decoded, or
// that seen records the object already has.
func (w *walker) key(known []string, seen *uint, parent string) (string, error) {
	tok, at, err := w.next()
	if err != nil {
		return "", err
	}

	// An object key is always a string token.
	key, _ := tok.(string)

	what := "a key"
	if parent != "" {
		what = "a key of " + text.Quote(parent)
	}

	if err := w.checkString(what, key, at); err != nil {
		return "", err
	}

	i := slices.Index(known, key)
	if i < 0 {
		hint := ""
		if j := slices.IndexFunc(known, func(k string) bool { return strings.EqualFold(k, key) }); j >= 0 {
			hint = " (keys are exact: did you mean " + text.Quote(known[j]) + "?)"
		}

		return "", fmt.Errorf("%w: unknown key %s at %s%s", ErrDecode, text.Quote(key), w.where(parent, key, at), hint)
	}

	if *seen&(1<<i) != 0 {
		return "", fmt.Errorf("%w: repeated key %s at %s", ErrDecode, text.Quote(key), w.where(parent, key, at))
	}

	*seen |= 1 << i

	return key, nil
}

// version reads the value of the schemaVersion key. findVersion has found it
// to be 1, and a second schemaVersion key is refused as repeated before its value
// is read; it is checked again all the same, so the walk never relies on it.
func (w *walker) version() error {
	_, at, err := w.next()
	if err != nil {
		return err
	}

	// A version that is not a number is refused here, before the walk would
	// have to step over an object or an array.
	return checkVersion(w.data[at:w.dec.InputOffset()], "")
}

// text reads the value of key in the object at path parent: a string that
// escapes no lone surrogate, or null for an absent value.
func (w *walker) text(parent, key string) error {
	tok, at, err := w.next()
	if err != nil {
		return err
	}

	path := join(parent, key)

	switch s := tok.(type) {
	case string:
		return w.checkString(text.Quote(path), s, at)
	case nil:
		return nil
	}

	return w.wrongType(path, at, "a string")
}

// resources reads the resources list, counting its elements as it goes: the
// list is refused at its first element past MaxResources.
func (w *walker) resources() error {
	tok, at, err := w.next()
	if err != nil {
		return err
	}

	if tok == nil {
		// null is refused, as a missing list, once the document is decoded.
		return nil
	}

	if tok != json.Delim('[') {
		return w.wrongType(keyResources, at, "an array")
	}

	for i := 0; w.dec.More(); i++ {
		if i == MaxResources {
			return fmt.Errorf("%w: more than %d", ErrTooManyResources, MaxResources)
		}

		if err := w.resource(i); err != nil {
			return err
		}
	}

	_, _, err = w.next()

	return err
}

// resource reads element i of the resources list.
func (w *walker) resource(i int) error {
	path := element(keyResources, i)

	tok, at, err := w.next()
	if err != nil {
		return err
	}

	if tok != json.Delim('{') {
		return w.wrongType(path, at, "an object")
	}

	var seen uint

	for w.dec.More() {
		key, err := w.key(resourceKeys[:], &seen, path)
		if err != nil {
			return err
		}

		if key == keyCapabilities {
			err = w.capabilities(join(path, key))
		} else {
			err = w.text(path, key)
		}

		if err != nil {
			return err
		}
	}

	_, _, err = w.next()

	return err
}

// capabilities reads the capabilities list at path: an array of at most
// MaxCapabilities strings, or null for an absent list. The list is refused at
// its first element past the cap, before that element is read.
func (w *walker) capabilities(path string) error {
	tok, at, err := w.next()
	if err != nil {
		return err
	}

	if tok == nil {
		return nil
	}

	if tok != json.Delim('[') {
		return w.wrongType(path, at, "an array")
	}

	for j := 0; w.dec.More(); j++ {
		if j == MaxCapabilities {
			at := skipSeparators(w.data, int(w.dec.InputOffset()))

			return fmt.Errorf("%w: %s lists more than %d capabilities, at %s",
				ErrDecode, text.Quote(path), MaxCapabilities, position(w.data, at))
		}

		tok, at, err := w.next()
		if err != nil {
			return err
		}

		s, ok := tok.(string)
		if !ok {
			return w.wrongType(element(path, j), at, "a string")
		}

		if err := w.checkString(text.Quote(element(path, j)), s, at); err != nil {
			return err
		}
	}

	_, _, err = w.next()

	return err
}

// checkString refuses s, a string token that starts at index at of the
// document, if its JSON text escapes a lone surrogate: a high surrogate escape
// (D800 to DBFF) not followed at once by a low one (DC00 to DFFF), or a low one
// on its own. Such an escape names no character, and encoding/json decodes it
// to U+FFFD, so the value would differ from the file. what names the string in
// the error. The JSON text is scanned only when s holds U+FFFD, which every
// lone surrogate decodes to; checkEncoding has refused invalid UTF-8 already.
func (w *walker) checkString(what, s string, at int) error {
	if !strings.ContainsRune(s, utf8.RuneError) {
		return nil
	}

	raw := w.data[at:w.dec.InputOffset()]

	i, unit := loneSurrogate(raw)
	if i < 0 {
		return nil
	}

	return fmt.Errorf("%w: %s escapes U+%04X, a surrogate on its own, which is no character, at %s",
		ErrDecode, what, unit, position(w.data, at+i))
}

// escapeLen is the length of a JSON escape of one UTF-16 code unit: a
// backslash, u and four hexadecimal digits.
const escapeLen = 6

// loneSurrogate returns the index in raw, the JSON text of a well-formed
// string, of the first escape of a lone surrogate, and the surrogate, or -1 if
// there is none. It steps over every other escape, so an escaped backslash
// followed by u is never read as an escape of its own.
func loneSurrogate(raw []byte) (int, rune) {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}

		r, ok := escapedUnit(raw, i)
		if !ok {
			// Any other escape is two bytes: step over the escaped byte.
			i++

			continue
		}

		if utf16.IsSurrogate(r) {
			// A pair decodes to one character; anything else is U+FFFD.
			if low, ok := escapedUnit(raw, i+escapeLen); !ok || utf16.DecodeRune(r, low) == unicode.ReplacementChar {
				return i, r
			}

			i += escapeLen
		}

		i += escapeLen - 1
	}

	return -1, 0
}

// escapedUnit returns the UTF-16 code unit escaped at index i of raw, and
// whether raw holds such an escape there: a backslash, u and four hexadecimal
// digits.
func escapedUnit(raw []byte, i int) (rune, bool) {
	if i+escapeLen > len(raw) || raw[i] != '\\' || raw[i+1] != 'u' {
		return 0, false
	}

	unit, err := strconv.ParseUint(string(raw[i+2:i+escapeLen]), 16, 16)
	if err != nil {
		return 0, false
	}

	return rune(unit), true
}

// wrongType reports that the value at path, which starts at index at of the
// document, is not of the JSON type want. The walk stops there, so a value that
// is an object or an array is never stepped over.
func (w *walker) wrongType(path string, at int, want string) error {
	return fmt.Errorf("%w: %s is %s, want %s, at %s", ErrDecode, text.Quote(path), kindOf(w.data[at]), want, position(w.data, at))
}

// where names the place of key in the object at path parent ("" for the
// document), starting at index at of the document: the key's path, unless the
// key is at the top level and its path is the key itself, then the line and
// column.
func (w *walker) where(parent, key string, at int) string {
	if parent == "" {
		return position(w.data, at)
	}

	return text.Quote(join(parent, key)) + ", " + position(w.data, at)
}

// join returns the path of key in the object at path parent, "" for the
// document: resources[1].regoin.
func join(parent, key string) string {
	if parent == "" {
		return key
	}

	return parent + "." + key
}

// element returns the path of element i of the array at path parent, with the
// index in brackets as the domain writes it: resources[1].
func element(parent string, i int) string {
	return parent + "[" + strconv.Itoa(i) + "]"
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

// skipSeparators is skipSpace that also steps over the comma between two
// values and the colon after a key, which json.Decoder.Token never returns: the
// result is where the next token starts.
func skipSeparators(data []byte, i int) int {
	for i < len(data) && (isSpace(data[i]) || data[i] == ',' || data[i] == ':') {
		i++
	}

	return min(i, len(data))
}

// isSpace reports whether b is JSON white space (RFC 8259, section 2).
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
