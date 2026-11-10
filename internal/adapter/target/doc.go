// Package target groups the driven adapters that describe what a target EU
// cloud provider offers, translated into the domain catalog. Each provider
// lives in its own sub-package (ovhcloud, scaleway, upcloud) and must pass the
// contract suite in targettest.
//
// A provider whose catalog is an embedded, dated snapshot keeps it as
// catalog.json beside a SOURCES.md in its own package, and reads it through the
// shared reader in the snapshot sub-package: one format, one strictness and one
// wording of errors for every target (ADR 0007). Scaleway and OVHcloud serve
// their catalogs this way. The provider package adds only
// what is its own, such as its display name in errors and its contract
// expectations. The lint rule target-adapters-read-snapshots-through-the-reader
// keeps encoding/json out of every package here but the reader, so a provider
// cannot decode its snapshot on its own. The lint rule
// embedded-snapshot-targets-stay-offline keeps io/fs, io/ioutil, net, os,
// plugin, syscall and unsafe, with their subpackages, out of every provider
// that serves an embedded snapshot, so it reads only the bytes it embeds; a
// provider that starts serving one is added to that rule by name.
package target
