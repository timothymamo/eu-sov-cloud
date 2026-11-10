// Package targettest provides the contract test suite every target adapter
// must pass, in the style of testing/fstest, so catalogs from different EU
// providers are comparable.
//
// An adapter test calls Run with its analysis.CatalogSource and the
// Expectations it declares for itself: its display name, the EU regions its
// offerings may be in and the taxonomy categories its catalog must serve. Run
// wraps Check, which returns every violation joined in one error, each naming
// the check it breaks (catalog, target, snapshot, regions, category, repeat,
// cancel), so the suite itself is tested against fakes that violate it on
// purpose.
//
// The package is imported only from tests, but is not itself a _test package:
// adapters in other packages share it.
package targettest
