package snapshot

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

	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// walker reads data, a well-formed JSON object, token by token, and builds the
// document from what it reads. A json.Decoder reading tokens holds one token at
// a time.
//
// encoding/json decodes keys ignoring case (and Unicode folding maps the long s
// to s), keeps the last of a repeated key, reads null and a missing field as the
// zero value, and decodes an escaped lone surrogate to U+FFFD; walker reads the
// document as it is written, character for character once JSON escapes are
// decoded, and its errors say where in the document the problem is, as a line
// and a column.
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
		// json.Valid has checked the syntax already, so the decoder fails here
		// only if the walker has lost its place.
		return nil, at, fmt.Errorf("%w: the JSON document cannot be read at %s", ErrDecode, position(w.data, at))
	}

	return tok, at, nil
}

// document walks the whole document, a JSON object whose exact schemaVersion
// key findVersion found to be 1, and returns what it holds.
func (w *walker) document() (document, error) {
	var doc document

	err := w.object("", documentKeys[:], func(key string) error {
		var err error

		switch key {
		case keySchemaVersion:
			err = w.version()
		case keyTarget:
			doc.target, err = w.text(keyTarget)
		case keySourceURL:
			doc.sourceURL, err = w.text(keySourceURL)
		case keyRetrievedOn:
			doc.retrievedOn, err = w.text(keyRetrievedOn)
		case keyRegions:
			doc.regions, err = list[string](w, keyRegions)
		case keyOfferings:
			doc.offerings, err = w.offerings()
		}

		return err
	})
	if err != nil {
		return document{}, err
	}

	return doc, nil
}

// object reads the object that starts at the next token, at path parent (""
// for the document), whose keys are known and all required. For each key, which
// must be spelt exactly as one of known and appear once, it calls value to read
// the key's value. Once the object closes, a key of known that it lacks is an
// error.
func (w *walker) object(parent string, known []string, value func(key string) error) error {
	tok, start, err := w.next()
	if err != nil {
		return err
	}

	if tok != json.Delim('{') {
		return w.wrongType(parent, start, "an object")
	}

	var seen uint

	for w.dec.More() {
		key, err := w.key(known, &seen, parent)
		if err != nil {
			return err
		}

		if err := value(key); err != nil {
			return err
		}
	}

	if _, _, err := w.next(); err != nil {
		return err
	}

	for i, key := range known {
		if seen&(1<<i) != 0 {
			continue
		}

		if parent == "" {
			return fmt.Errorf("%w: required key %s is missing from the document", ErrDecode, text.Quote(key))
		}

		return fmt.Errorf("%w: required key %s is missing from %s, which starts at %s",
			ErrDecode, text.Quote(key), text.Quote(parent), position(w.data, start))
	}

	return nil
}

// key reads the next key of an object at path parent ("" for the document),
// whose keys are known, and refuses a key that escapes a lone surrogate, that is
// not character for character one of them once JSON escapes are decoded, or
// that seen records the object already has.
func (w *walker) key(known []string, seen *uint, parent string) (string, error) {
	tok, at, err := w.next()
	if err != nil {
		return "", err
	}

	// An object key is always a string token.
	key, _ := tok.(string)

	what := func() string {
		if parent == "" {
			return "a key"
		}

		return "a key of " + text.Quote(parent)
	}

	if err := w.checkString(key, at, what); err != nil {
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

// version reads the value of the schemaVersion key. findVersion has found it to
// be 1, and a second schemaVersion key is refused as repeated before its value is
// read; it is checked again all the same, so the walk never relies on it.
func (w *walker) version() error {
	_, at, err := w.next()
	if err != nil {
		return err
	}

	// A version that is not a number is refused here, before the walk would
	// have to step over an object or an array.
	return checkVersion(w.data[at:w.dec.InputOffset()], "")
}

// text reads the value at path: a string that escapes no lone surrogate. null is
// refused like any other value that is not a string: the format has no optional
// field.
func (w *walker) text(path string) (string, error) {
	return w.str(func() string { return path })
}

// str does the work of text. It calls path only to report an error, so the path
// of a list element is built, and quoted, only for an element at fault: a list
// may hold a hundred thousand strings.
func (w *walker) str(path func() string) (string, error) {
	tok, at, err := w.next()
	if err != nil {
		return "", err
	}

	s, ok := tok.(string)
	if !ok {
		return "", w.wrongType(path(), at, "a string")
	}

	if err := w.checkString(s, at, func() string { return text.Quote(path()) }); err != nil {
		return "", err
	}

	return s, nil
}

// list reads the value at path: an array of strings, each escaping no lone
// surrogate, as values of type T. An empty array is an empty, non-nil list; null
// is refused, since it says nothing the document means.
func list[T ~string](w *walker, path string) ([]T, error) {
	tok, at, err := w.next()
	if err != nil {
		return nil, err
	}

	if tok != json.Delim('[') {
		return nil, w.wrongType(path, at, "an array of strings")
	}

	values := []T{}

	for i := 0; w.dec.More(); i++ {
		s, err := w.str(func() string { return element(path, i) })
		if err != nil {
			return nil, err
		}

		values = append(values, T(s))
	}

	if _, _, err := w.next(); err != nil {
		return nil, err
	}

	return values, nil
}

// offerings reads the offerings list: an array of offering objects, each with
// every key of offeringKeys.
func (w *walker) offerings() ([]offering, error) {
	tok, at, err := w.next()
	if err != nil {
		return nil, err
	}

	if tok != json.Delim('[') {
		return nil, w.wrongType(keyOfferings, at, "an array of objects")
	}

	offerings := []offering{}

	for i := 0; w.dec.More(); i++ {
		o, err := w.offering(element(keyOfferings, i))
		if err != nil {
			return nil, err
		}

		offerings = append(offerings, o)
	}

	if _, _, err := w.next(); err != nil {
		return nil, err
	}

	return offerings, nil
}

// offering reads the offering object at path.
func (w *walker) offering(path string) (offering, error) {
	var o offering

	err := w.object(path, offeringKeys[:], func(key string) error {
		var err error

		at := join(path, key)

		switch key {
		case keyName:
			o.name, err = w.text(at)
		case keyCategories:
			o.categories, err = list[taxonomy.Category](w, at)
		case keyCapabilities:
			o.capabilities, err = list[taxonomy.Capability](w, at)
		case keyRegions:
			o.regions, err = list[string](w, at)
		}

		return err
	})
	if err != nil {
		return offering{}, err
	}

	return o, nil
}

// checkString refuses s, a string token that starts at index at of the
// document, if its JSON text escapes a lone surrogate: a high surrogate escape
// (D800 to DBFF) not followed at once by a low one (DC00 to DFFF), or a low one
// on its own. Such an escape names no character, and encoding/json decodes it to
// U+FFFD, so the value would differ from the document. what names the string in
// the error and is called only then. The JSON text is scanned only when s holds
// U+FFFD, which every lone surrogate decodes to; decode has refused invalid
// UTF-8 already.
func (w *walker) checkString(s string, at int, what func() string) error {
	if !strings.ContainsRune(s, utf8.RuneError) {
		return nil
	}

	raw := w.data[at:w.dec.InputOffset()]

	i, unit := loneSurrogate(raw)
	if i < 0 {
		return nil
	}

	return fmt.Errorf("%w: %s escapes U+%04X, a surrogate on its own, which is no character, at %s",
		ErrDecode, what(), unit, position(w.data, at+i))
}

// escapeLen is the length of a JSON escape of one UTF-16 code unit: a
// backslash, u and four hexadecimal digits.
const escapeLen = 6

// loneSurrogate returns the index in raw, the JSON text of a well-formed string,
// of the first escape of a lone surrogate, and the surrogate, or -1 if there is
// none. It steps over every other escape, so an escaped backslash followed by u
// is never read as an escape of its own.
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
// document, is not of the JSON type want. null is a wrong type like any other:
// the format has no optional field. The walk stops there, so a value that is an
// object or an array is never stepped over.
func (w *walker) wrongType(path string, at int, want string) error {
	if path == "" {
		return fmt.Errorf("%w: the document is %s, want %s, at %s", ErrDecode, kindOf(w.data[at]), want, position(w.data, at))
	}

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

// join returns the path of key in the object at path parent: offerings[1].name.
func join(parent, key string) string {
	return parent + "." + key
}

// element returns the path of element i of the array at path parent, with the
// index in brackets as the domain writes it: offerings[1].
func element(parent string, i int) string {
	return parent + "[" + strconv.Itoa(i) + "]"
}

// skipSeparators is skipSpace that also steps over the comma between two values
// and the colon after a key, which json.Decoder.Token never returns: the result
// is where the next token starts.
func skipSeparators(data []byte, i int) int {
	for i < len(data) && (isSpace(data[i]) || data[i] == ',' || data[i] == ':') {
		i++
	}

	return min(i, len(data))
}
