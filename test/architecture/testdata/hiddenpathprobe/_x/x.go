// Package x sits in a directory whose name starts with _, which the go command
// skips when it walks ./... and builds when a package imports it by explicit
// path. The probe's cmd/shift2eu imports it, so the hidden-path check must
// report it (issue #206).
package x

// Name is what the probe's cmd/shift2eu prints, so the package is reached.
func Name() string { return "_x" }
