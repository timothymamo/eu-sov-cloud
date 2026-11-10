// Package jsondoc decodes a JSON document strictly, for the adapters that read
// one: the inventory-file reader (ADR 0006) and the target catalog snapshot
// reader (ADR 0007). It holds the rules both formats share, so they are written,
// worded and tested once (ADR 0009). Each format keeps its own decisions and
// drives the decoding through this package's API.
//
// Open checks a document as a whole and refuses it, in this order, if it is:
//
//   - longer than the caller's cap, in bytes;
//   - started by a byte order mark, UTF-8 (EF BB BF) or UTF-16 (FF FE, FE FF),
//     which is refused by name;
//   - not valid UTF-8: encoding/json would read such a byte as U+FFFD, so a
//     value would differ from the document without a word;
//   - not one JSON value with nothing after it but white space: an empty
//     document, one cut short, malformed JSON, or "unexpected data after the
//     first JSON value";
//   - a value that is not an object.
//
// Document.Lookup then finds the value of one top-level key, spelt exactly, before
// anything else is checked, which is how a format reads its schema version
// first. Document.Walker reads the document token by token: Walker.Object reads
// an object whose keys a format lists as Key values and calls back for each key,
// and Walker.Array, Walker.String, Walker.Null and Walker.Raw read the values a
// callback finds. As it reads, the walker refuses:
//
//   - a key that is not character for character one of the keys the format
//     lists, once JSON escapes are decoded (encoding/json matches keys ignoring
//     case, and Unicode folding maps the long s to s), with a hint when it
//     equals one ignoring case;
//   - a key the object already has (encoding/json keeps the last of them);
//   - a value of the wrong JSON type, null included: null is a value only where
//     a format calls Walker.Null first;
//   - an object without a key the format marks Required;
//   - a string, key or value, that escapes a surrogate (U+D800 to U+DFFF) other
//     than as a high surrogate followed at once by a low one: such an escape
//     names no character, and encoding/json decodes it to U+FFFD.
//
// The walker reads one token at a time, and a format builds its values as it
// goes, so nothing is decoded twice and a format can refuse a list at its first
// element past a cap before that element is read.
//
// Each format decides which keys exist and which are required, its null policy
// per field (Walker.Null), its caps and counts (the size cap it passes to Open,
// and any count it checks in a callback), whether and how it reads a key first
// (its schema version), its sentinels, and the wording of its own errors.
//
// Errors are one line. Every error about the document wraps the sentinel the
// format passes to Open, and an error a callback returns is passed on as it is,
// so a format reports its own sentinels (a schema version, a count past a cap)
// through the walk. A document error is worded the same for every format,
// speaks of "the document" and never names the input: the format adds that. It
// places the problem by line and column, both counted from 1, the line by line
// feeds and the column in characters (a tab or a multi-byte character counts
// once), and a malformed multi-byte character at its first byte, except where
// it speaks of the whole document (the size cap, a byte order mark, an empty or
// cut-short document, a document that is not an object, a missing top-level
// key). A value is named by its path, keys joined by dots and an array index in
// brackets as the domain writes it (resources[1].capabilities[0]). Values from
// the document are shown only through text.Quote, and the text of encoding/json
// errors is never passed on. An error that does not wrap the format's sentinel
// and starts with "jsondoc:" is a mistake in the format's use of this package,
// not in the document.
//
// Adapter layer, shared by adapters: parses bytes it is given, opens no file and
// makes no network call. Go's internal rule keeps it to packages under
// internal/adapter.
package jsondoc
