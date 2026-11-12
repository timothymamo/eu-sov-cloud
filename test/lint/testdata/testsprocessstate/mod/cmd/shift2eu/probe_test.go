// The CLI composition root's _test.go file for TestTestsNoProcessStateProbe
// (issue #153): the group tests-no-process-state holds for the roots' tests as it
// holds for the application layer's, so the rule is the same on both sides of the
// port. The composition roots' forbidigo groups already refuse the environment
// the root reads; a root's test that sets one changes what the root sees, and
// t.Chdir moves the working directory the run resolves a --inventory path
// against. Nothing here runs: the probe lints the file.
package main

import "testing"

// TestProbe holds the calls the group refuses on a *testing.T.
func TestProbe(t *testing.T) {
	t.Setenv("SHIFT2EU_TOKEN", "example") // want "tests-no-process-state: "
	t.Chdir("example")                    // want "tests-no-process-state: "

	t.TempDir()
}
