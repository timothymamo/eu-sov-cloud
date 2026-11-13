// The domain layer's _test.go file for TestTestsNoProcessStateProbe (issue #153,
// the review's finding S2): the domain is pure in its production files, and its
// tests are no exception to the process-state rule either — the group holds for
// every test of the module but the two harnesses under test/. A domain test that
// sets a variable or changes the working directory does so for the whole test
// binary, every other package's tests included.
//
// Nothing here runs: the probe lints the file.
package taxonomy_test

import "testing"

// TestProbe holds the calls the group refuses on a *testing.T.
func TestProbe(t *testing.T) {
	t.Setenv("SHIFT2EU_TOKEN", "example") // want "tests-no-process-state: "
	t.Chdir("example")                    // want "tests-no-process-state: "

	t.TempDir()
}
