// Package testutil stands in for the shared test helpers, for
// TestTestutilImportsProbe: testutil-imports holds every file of internal/testutil
// to the imports the helpers make today, and to nothing else (issue #144). An
// application layer's test may import this package, so a helper that reached the
// network, started a program or read the environment would hand a test I/O with
// lint green; the rule refuses the import and the forbidigo group testutil-no-env
// refuses the call.
//
// This file imports nothing, so the package builds while the probe's refusals are
// read. golden.go imports what the helpers really import, with no want comment:
// those imports pass.
package testutil

// Fake stands for a fake an application test hands the use case.
func Fake() {}
