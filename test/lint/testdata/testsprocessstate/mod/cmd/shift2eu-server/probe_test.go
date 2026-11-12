// The server composition root's _test.go file for TestTestsNoProcessStateProbe
// (issue #153): the same calls as the CLI's, because a rule that held one root's
// tests and not the other's would leave the server open. A test that sets the
// variable the server reads its token from, or moves the working directory the
// run resolves a path against, changes what every test in the same process sees.
// Nothing here runs: the probe lints the file.
package main

import "testing"

// TestProbe holds the calls the group refuses on a *testing.T.
func TestProbe(t *testing.T) {
	t.Setenv("SHIFT2EU_SERVER_TOKEN", "example") // want "tests-no-process-state: "
	t.Chdir("example")                           // want "tests-no-process-state: "

	t.TempDir()
}
