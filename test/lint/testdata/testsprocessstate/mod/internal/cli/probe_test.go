// The driving adapter's _test.go file for TestTestsNoProcessStateProbe (issue
// #153): the group tests-no-process-state holds for internal/cli's tests, which
// run in the same process as the CLI and the composition root that hands it the
// work. t.Setenv sets a variable every test in the process then sees, and
// t.Chdir moves the directory the run resolves --inventory and --output against.
// Nothing here runs: the probe lints the file.
package cli_test

import "testing"

// TestProbe holds the calls the group refuses on a *testing.T.
func TestProbe(t *testing.T) {
	t.Setenv("SHIFT2EU_TOKEN", "example") // want "tests-no-process-state: "
	t.Chdir("example")                    // want "tests-no-process-state: "

	t.TempDir()
}
