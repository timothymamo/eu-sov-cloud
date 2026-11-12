// Package testutil stands in for the shared test helpers: the rule names
// internal/testutil in its allow list, so an application-layer test may import
// it, and testutil-only-in-tests keeps it out of production files.
package testutil

// Fake stands for a fake an application test hands the use case.
func Fake() {}
