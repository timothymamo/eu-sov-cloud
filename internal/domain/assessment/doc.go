// Package assessment turns the mapping of every resource in an inventory onto a
// target's catalog into a migration assessment: a verdict per resource (its fit,
// best offering, reasons and the capabilities it must do without), the blockers
// that stop resources with fit none from moving, and summary counts per fit.
// Fits and reasons come from package mapping unchanged.
//
// Compare puts several targets' assessments of one inventory side by side, in
// target key order, and finds the shared blockers: the resources with fit none
// on every target. It ranks no target.
//
// What every report format shares lives here, so the formats cannot drift
// apart: Verdict.Ties names the other candidates a reader could take for the
// best one, Verdict.ChangesToList gives the required changes a report lists
// (those of a partial fit only), and
// Comparison.Validate the preconditions a comparison must meet before any
// reporter lays it out (the same resources in the same order on every target,
// and shared blockers that name only compared targets).
//
// Domain layer: standard library and other domain packages only, no I/O, no
// clock.
package assessment
