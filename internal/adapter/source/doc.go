// Package source groups the driven adapters that produce the domain inventory
// of a source hyperscaler. inventoryfile reads it from a JSON inventory file
// (ADR 0006); each provider's live discovery lives in its own sub-package (aws,
// azure, gcp). Every one of them must pass the contract suite in sourcetest.
//
// Source adapters are strictly read-only: they never create, modify or delete
// cloud resources.
package source
