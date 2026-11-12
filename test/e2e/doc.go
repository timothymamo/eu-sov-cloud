// Package e2e holds black-box tests that build the shift2eu binary and run it
// against the fixtures in testdata. Test files here carry the e2e build tag and
// run with `make test-e2e`. Every target the binary offers is assessed with the
// same fixture, each against a golden report of its own, so the reports can be
// compared in shape and a target added later gets the same coverage.
package e2e
