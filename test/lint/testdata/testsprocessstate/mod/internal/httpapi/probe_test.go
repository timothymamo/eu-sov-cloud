// The HTTP API's _test.go file for TestTestsNoProcessStateProbe (issue #153): the
// group tests-no-process-state holds for internal/httpapi's tests, which run in
// the same process as the server that serves them. t.Setenv sets a variable every
// test in the process then sees, and t.Chdir moves the working directory the run
// resolves a path against. Nothing here runs: the probe lints the file.
package httpapi_test

import "testing"

// TestProbe holds the calls the group refuses on a *testing.T.
func TestProbe(t *testing.T) {
	t.Setenv("SHIFT2EU_TOKEN", "example") // want "tests-no-process-state: "
	t.Chdir("example")                    // want "tests-no-process-state: "

	t.TempDir()
}
