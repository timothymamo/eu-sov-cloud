// Package text validates and safely quotes the untrusted free text that domain
// values carry: resource IDs, raw types, regions, target and offering names.
//
// Such text comes from inventory files and catalog snapshots and ends up in
// reports and terminals. Validate rejects what would corrupt either (control
// characters, invisible formatting characters such as bidi overrides, the
// Unicode line and paragraph separators U+2028 and U+2029, invalid UTF-8,
// over-long values) instead of rendering it. It also rejects look-alike
// characters that render as nothing or as a blank, so two values that differ
// only by one would look the same in a report (variation selectors, the other
// default-ignorable code points such as the Hangul fillers, U+2800 BRAILLE
// PATTERN BLANK), noncharacters, which are never text, and leading or trailing
// white space, which pads a value to look like another. Values are never
// trimmed: they are valid as given or rejected. Look-alike letters (homoglyphs
// such as Cyrillic а for Latin a) and Unicode normalisation (NFC and NFD
// spellings of the same text) are not detected. Quote turns any
// string, valid or not, into a short, escaped form that is safe to echo in an
// error message.
//
// Escaping for an output format, such as a Markdown table cell, is not done here:
// that belongs to the reporter for that format.
//
// Domain layer: standard library only, no I/O.
package text
