// Package assessment turns the mapping of every resource in an inventory onto a
// target's catalog into a migration assessment: a verdict per resource (its fit,
// best offering, reasons and the capabilities it must do without), the blockers
// that stop resources with fit none from moving, and summary counts per fit.
// Fits and reasons come from package mapping unchanged.
//
// Domain layer: standard library and other domain packages only, no I/O.
package assessment
