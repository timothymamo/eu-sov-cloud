// Package source groups the driven adapters that discover infrastructure in a
// source hyperscaler and translate it into a domain inventory. Each provider
// lives in its own sub-package (aws, azure, gcp) and must pass the contract
// suite in sourcetest.
//
// Source adapters are strictly read-only: they never create, modify or delete
// cloud resources.
package source
