// Package apiparity holds the offline parity test between the two driving
// adapters of the assess path: internal/cli and internal/httpapi (PRD #103,
// ADR 0019). It is test infrastructure, not a layer: it imports the real
// inventory, target and report adapters and wires them into both, as a
// composition root wires them, which no package of internal/ may do. The
// package has no production code, so it is part of no binary and holds nothing
// to a release target; only its test files carry the test.
//
// The test runs without the network, without credentials and without a build
// tag, so `make test` runs it with every other default test. It copies a
// sanitised inventory fixture to a temporary directory as inventory.json and
// asserts that `shift2eu assess` and POST /v1/assessments answer the same bytes
// for the same inventory and targets, and that the JSON body matches
// jsonreport's schema.
package apiparity
