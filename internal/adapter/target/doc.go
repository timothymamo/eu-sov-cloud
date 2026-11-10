// Package target groups the driven adapters that describe what a target EU
// cloud provider offers, translated into the domain catalog. Each provider
// lives in its own sub-package (ovhcloud, scaleway, upcloud) and must pass the
// contract suite in targettest.
//
// A provider whose catalog is an embedded, dated snapshot keeps it as
// catalog.json beside a SOURCES.md in its own package, and reads it through the
// shared reader in the snapshot sub-package: one format, one strictness and one
// wording of errors for every target (ADR 0007). The provider package adds only
// what is its own, such as its display name in errors and its contract
// expectations.
package target
