// Package snapshot reads a target catalog snapshot: the dated JSON document,
// schema version 1, that a target adapter embeds to describe what its target
// offers in the EU regions it declares (ADR 0007). Every target adapter with an
// embedded snapshot reads it through Parse, so the format, its strictness and its
// errors are the same for every target and are pinned once, here.
//
// A snapshot holds the target's display name, the source URL and retrieval date
// (retrievedOn, YYYY-MM-DD) of its data, the EU regions it declares, and its
// offerings, each with a name, taxonomy categories, capabilities and regions.
// Every field is required, at the top level and in each offering; a list may be
// empty.
//
// Parse refuses a document, with an error, unless all of these hold, checked in
// this order:
//
//   - it is at most MaxDocumentBytes long, checked before anything else;
//   - it starts with no byte order mark, UTF-8 or UTF-16, which is refused by
//     name as the inventory file reader does (ADR 0006), and it is valid UTF-8,
//     both checked before any decoding, since the JSON decoder would read a
//     byte that is not UTF-8 as U+FFFD;
//   - it is one JSON object with nothing after it but white space;
//   - its schemaVersion is the integer 1, found before any other key or value is
//     checked, so a document of another version is refused for its version
//     alone;
//   - read token by token: every key is spelt exactly as the format spells it,
//     character for character once JSON escapes are decoded, and appears once
//     per object; every field the format defines is present, not null, and of
//     its JSON type; and no string, key or value, escapes a lone surrogate;
//   - its retrievedOn is a YYYY-MM-DD date;
//   - every declared region is non-blank, valid text (text.Validate) and
//     declared once, and every offering region is a declared one;
//   - catalog.New accepts its target, its source URL and every offering's name,
//     categories, capabilities and regions.
//
// The token walk is what encoding/json alone does not give: it matches keys
// without regard to case, keeps the last of a repeated key, reads null and a
// missing field as the zero value, and decodes an escaped lone surrogate to
// U+FFFD, so in each case the catalog would differ from the document without a
// word.
//
// Every error is one line and wraps ErrDecode, ErrSchemaVersion,
// ErrUndeclaredRegion or the error of catalog.New. A problem is placed by line
// and column and a value named by its path (offerings[1].regions[0]); values
// from the document are shown through text.Quote, and the text of encoding/json
// and time.Parse errors is never passed on. Errors name neither the input nor
// any target: the adapter that embeds the document does.
//
// Every snapshot sits beside a SOURCES.md that gives each offering a section of
// its own, naming the pages it comes from, the fact behind each category,
// capability and region, and the licence of each source. This package's tests
// check that rule for every target adapter's catalog.json.
//
// Adapter layer: parses bytes it is given, opens no file and makes no network
// call.
package snapshot
