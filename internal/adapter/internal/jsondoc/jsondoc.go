package jsondoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// ErrMisuse is wrapped by every error that reports a mistake in a format's use
// of this package, not a problem with the document: a callback that returns
// without reading the value it was called for, or reads it a second time; a
// list of keys longer than MaxKeys or that holds a Name twice; a nil sentinel;
// the zero Document; and a walker read after the walk has ended. Such an error
// starts with "jsondoc:" and never wraps the format's sentinel. Reading a
// document, however malformed, never causes one: it is a bug in the format.
var ErrMisuse = errors.New("jsondoc: misused by the format")

// Document is a JSON document Open has checked as a whole: at most the caller's
// cap, UTF-8 without a byte order mark, and one JSON object with nothing after
// it but white space. Its keys and values are checked as a Walker reads them.
// The zero Document holds no document; use Open.
type Document struct {
	data    []byte
	invalid error
}

// byteOrderMarks are U+FEFF encoded as UTF-8, UTF-16LE and UTF-16BE. They are
// never modified.
var byteOrderMarks = [...]string{"\xef\xbb\xbf", "\xff\xfe", "\xfe\xff"}

// Open checks data as a whole and returns it as a Document. It refuses, in this
// order: more than maxBytes bytes; a byte order mark, UTF-8 or UTF-16, at the
// start; a byte that is not UTF-8; anything but one JSON value with nothing
// after it but white space; and a value that is not an object.
//
// Every error wraps invalid, the format's sentinel for a document it cannot
// read, and is worded as the package documentation says; a nil invalid is
// refused as ErrMisuse. Open keeps data, not a copy: the caller must not change
// it while the Document is in use.
func Open(data []byte, maxBytes int, invalid error) (Document, error) {
	if invalid == nil {
		return Document{}, errNilSentinel("Open")
	}

	if len(data) > maxBytes {
		return Document{}, fmt.Errorf("%w: the document is %d bytes, more than the limit of %d",
			invalid, len(data), maxBytes)
	}

	for _, bom := range byteOrderMarks {
		if bytes.HasPrefix(data, []byte(bom)) {
			return Document{}, fmt.Errorf("%w: the document starts with a byte order mark; save it as UTF-8 without one",
				invalid)
		}
	}

	if !utf8.Valid(data) {
		return Document{}, fmt.Errorf("%w: the document is not valid UTF-8 at %s",
			invalid, position(data, invalidUTF8(data)))
	}

	if !json.Valid(data) {
		return Document{}, fmt.Errorf("%w: %s", invalid, syntaxReason(data))
	}

	// json.Valid has refused a document that is only white space.
	if start := skipSpace(data, 0); data[start] != '{' {
		return Document{}, fmt.Errorf("%w: the document is %s, not a JSON object", invalid, kind(data[start]))
	}

	return Document{data: data, invalid: invalid}, nil
}

// Lookup returns the JSON text of the value of the document object's first key
// spelt exactly key, as the document spells it (the number 1 is "1", the string
// "1" is "\"1\""), or nil if the object has no such key. folded is a key of the
// object that equals key ignoring case (strings.EqualFold), or "" if there is
// none, so a format can name the key that was meant: the first such key before
// the exact key, or, when the object has no exact key, the first anywhere in
// the object.
//
// Lookup reads the object's keys only, steps over every other value and stops
// at key. It checks nothing else, so a format calls it before Walker to read a
// key, such as its schema version, before any other key or value is checked.
// On the zero Document it returns an error that wraps ErrMisuse.
func (d Document) Lookup(key string) (raw []byte, folded string, err error) {
	w := d.Walker()

	if err := w.checkDocument(); err != nil {
		return nil, "", err
	}

	if _, _, err := w.next(); err != nil {
		return nil, "", err
	}

	for w.dec.More() {
		tok, at, err := w.next()
		if err != nil {
			return nil, "", err
		}

		k, ok := tok.(string)
		if !ok {
			return nil, "", w.lost(at)
		}

		if k != key && folded == "" && strings.EqualFold(k, key) {
			folded = k
		}

		start, err := w.skip()
		if err != nil {
			return nil, "", err
		}

		if k == key {
			return d.data[start:w.dec.InputOffset()], folded, nil
		}
	}

	return nil, folded, nil
}

// errNilSentinel reports that a format passed fn a nil sentinel, which every
// error fn returns would fail to wrap.
func errNilSentinel(fn string) error {
	return fmt.Errorf("%w: %s needs the format's sentinel, not nil", ErrMisuse, fn)
}

// Walker returns a Walker at the start of the document: the first value it reads
// is the document's object, which Walker.Object reads. Each Walker reads the
// document once, from the start. The Walker of the zero Document refuses its
// first read with an error that wraps ErrMisuse.
func (d Document) Walker() *Walker {
	dec := json.NewDecoder(bytes.NewReader(d.data))
	dec.UseNumber()

	return &Walker{data: d.data, dec: dec, invalid: d.invalid}
}

// Describe names the JSON value whose text is raw, for an error message: an
// integer that fits an int and is written as one is shown as written (2, -1);
// any other number, an integer too long for an int included, as written,
// through text.Quote, after "the number", so -0, 1.0 and 1e0 are never shown as
// 0 or 1, values the document does not hold; a string by its value, JSON
// escapes decoded, through text.Quote, after "the string", with U+FFFD where it
// escapes a lone surrogate, as encoding/json decodes one; and any other value
// by its type, with its article: "an object", "an array", "a boolean" or
// "null". raw is one well-formed JSON value, as Lookup and Walker.Raw return it;
// an empty raw is "no value".
func Describe(raw []byte) string {
	if len(raw) == 0 {
		return "no value"
	}

	if n, err := strconv.Atoi(string(raw)); err == nil && strconv.Itoa(n) == string(raw) {
		return string(raw)
	}

	switch k := kind(raw[0]); k {
	case "a string":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return k
		}

		return "the string " + text.Quote(s)
	case "a number":
		return "the number " + text.Quote(string(raw))
	default:
		return k
	}
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
	return "unexpected data after the first JSON value at " + position(data, skipSpace(data, int(dec.InputOffset())))
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

// kind names, with its article, the JSON type of the value whose first byte is
// b.
func kind(b byte) string {
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

// skipSeparators is skipSpace that also steps over the comma between two values
// and the colon after a key, which json.Decoder.Token never returns: the result
// is where the next token starts.
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
