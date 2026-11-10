package inventoryfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

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

// skipSeparators is skipSpace that also steps over the comma between two
// values and the colon after a key, which json.Decoder.Token never returns: the
// result is where the next token starts.
func skipSeparators(data []byte, i int) int {
	for i < len(data) && (isSpace(data[i]) || data[i] == ',' || data[i] == ':') {
		i++
	}

	return min(i, len(data))
}
