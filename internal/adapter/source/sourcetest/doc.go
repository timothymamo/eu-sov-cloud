// Package sourcetest provides the contract test suite every source adapter
// must pass, in the style of testing/fstest, so behaviour stays consistent
// across the inventory file, AWS, Azure and GCP.
//
// An adapter test calls Run with its analysis.InventorySource, built over
// recorded, sanitised input, and the inventory that input must produce. Run
// wraps Check, which returns every violation joined in one error, each naming
// the check it breaks (want, repeat, cancel), so the suite itself is tested
// against fakes that violate it on purpose. An adapter's own error is shown
// through text.Quote and stays in the violation's chain.
//
// The package is imported only from tests, but is not itself a _test package:
// adapters in other packages share it.
package sourcetest
