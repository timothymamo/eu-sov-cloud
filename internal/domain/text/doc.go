// Package text validates and safely quotes the untrusted free text that domain
// values carry: resource IDs, raw types, regions, target and offering names.
//
// Such text comes from inventory files and catalog snapshots and ends up in
// reports and terminals. Validate rejects what would corrupt either (control
// characters, invisible formatting characters such as bidi overrides, the
// Unicode line and paragraph separators U+2028 and U+2029, invalid UTF-8,
// over-long values) instead of rendering it. Quote turns any string, valid
// or not, into a short, escaped form that is safe to echo in an error message.
//
// Escaping for an output format, such as a Markdown table cell, is not done here:
// that belongs to the reporter for that format.
//
// Domain layer: standard library only, no I/O.
package text
