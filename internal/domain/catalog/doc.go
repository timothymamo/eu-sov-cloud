// Package catalog models what a target EU cloud provider (OVHcloud, Scaleway,
// UpCloud) offers: its offerings, the taxonomy categories and capabilities each
// one covers, the EU regions it is available in, and the dated, sourced snapshot
// the data was taken from.
//
// A Catalog is validated when it is built with New: a target display name that
// is not blank, a snapshot dated and sourced from an absolute https URL with a
// host name, no user info and RFC 3986 characters only, uniquely named
// offerings that each serve at least one known category in at least one region,
// known capabilities, and free text (target, offering names, regions) that
// passes text.Validate. Values are stored as given, never trimmed; the
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
