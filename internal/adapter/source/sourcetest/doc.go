// Package sourcetest provides the contract test suite every source adapter
// must pass, in the style of testing/fstest: an adapter test calls the suite
// with its implementation and recorded fixtures, so behaviour stays consistent
// across AWS, Azure and GCP.
package sourcetest
