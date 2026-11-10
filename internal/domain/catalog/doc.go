// Package catalog models what a target EU cloud provider (OVHcloud, Scaleway,
// UpCloud) offers: its offerings, the taxonomy categories and capabilities each
// one covers, the EU regions it is available in, and the dated, sourced snapshot
// the data was taken from.
//
// A Catalog is validated when it is built with New: a target display name that
// is not blank, a snapshot dated and sourced from an absolute https URL with an
// ASCII DNS host name (not an IP address, not percent-encoded), no user info, no
// square brackets, complete percent escapes, RFC 3986 characters only, no HTML
// character reference shape and percent escapes that decode to valid text,
// uniquely named offerings that each serve at least one known category in at
// least one region, known capabilities, and free text (target, offering names,
// regions) that passes text.Validate. Offering names are compared ignoring the
// length of runs of white space; look-alike letters and Unicode normalisation
// are not compared. Values are stored as given, never trimmed or decoded; the
// retrieval date is kept in UTC. Its offerings are kept in name order, each with
// its categories, capabilities and regions sorted and de-duplicated, so
// everything derived from it is deterministic. A zero Catalog has not been
// validated; later layers only receive catalogs built by New.
//
// The target is the display name reports show (for example Scaleway), not a CLI
// key: choosing an adapter from a key is the composition root's job.
//
// Domain layer: standard library and other domain packages only, no I/O.
package catalog
