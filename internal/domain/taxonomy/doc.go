// Package taxonomy defines the provider-neutral service categories (for example
// compute.vm, kubernetes.managed, storage.object, database.postgresql) that are
// the shared language between source hyperscalers and target EU providers.
//
// Source adapters classify discovered resources into these categories and
// target adapters declare which categories their offerings serve, so matching
// never needs a hand-written table for every provider pair.
//
// Domain layer: standard library only, no I/O.
package taxonomy
