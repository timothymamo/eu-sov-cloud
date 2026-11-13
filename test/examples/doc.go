// Package examples holds the tests that keep the sample inventories of the
// top-level examples/ directory honest (issue #179). A person trying shift2eu
// or watching a demo reads those samples, so they may not rot: every
// examples/**/*.json file is decoded by the real inventory-file adapter and
// declares the source its directory names, every sample assesses against every
// target through the real application layer and the embedded catalog snapshots,
// the samples together use every taxonomy category, every capability and every
// fit, they carry documentation values only (an AWS account ID of 123456789012,
// addresses in 192.0.2.0/24, names under example.com or .example), and the
// reports the walkthrough quotes are golden files regenerated with -update so a
// catalog snapshot change shows up in the diff.
//
// The package holds this file and _test.go files only, under the strict
// test-docs-are-pure depguard rule, so it imports nothing. Its tests run in make
// test, need no network, credentials or clock, and are table-driven and
// black-box (package examples_test).
package examples
