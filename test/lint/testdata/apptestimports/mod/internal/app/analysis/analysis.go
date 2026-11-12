// Package analysis stands for an application-layer use case. Its imports are a
// strict allow list (app-uses-ports-not-adapters), so this file imports nothing.
// The probe beside it is a test of the same layer, which
// app-tests-use-fakes-not-adapters holds to the shared deny list instead.
package analysis

// Assess stands for the use case the probe's test exercises.
func Assess() {}
