// Package analysis stands for an application-layer use case. Its imports are a
// strict allow list (app-uses-ports-not-adapters), so this file imports nothing.
// The probe tests beside it are tests of the same layer, which
// app-tests-use-fakes-not-adapters holds to a strict allow list of its own
// (issue #144).
package analysis

// Assess stands for the use case the probe's test exercises.
func Assess() {}
