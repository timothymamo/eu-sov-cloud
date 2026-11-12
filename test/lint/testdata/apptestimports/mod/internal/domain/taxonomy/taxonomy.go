// Package taxonomy stands in for a domain package: the application layer and
// its tests meet their providers only through the taxonomy (CLAUDE.md), so the
// probe's allow test imports it, and the rule allows it because it is one of
// the domain packages named in the allow list.
package taxonomy

// Category is a provider-neutral service type, as the domain names it.
type Category string

// StorageObject is one category of the real taxonomy.
const StorageObject Category = "storage.object"
