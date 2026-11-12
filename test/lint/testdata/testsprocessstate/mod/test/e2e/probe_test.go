// The e2e tier's _test.go file for TestTestsNoProcessStateProbe (issue #153, the
// review's finding S2): the tier builds the binary and runs it as a black box, so
// it needs no variable of its own set, and the group tests-no-process-state holds
// for every test of the module but the two harnesses under test/: a test that sets
// a variable here changes the environment of the binary the whole tier runs in,
// a credential's name among them (CLAUDE.md rule 3). A case passes its variables
// to the binary's environment as the command's Env, which is the fixture's value
// and no change to this process.
//
// Nothing here runs: the probe lints the file.
package e2e_test

import "testing"

// TestProbe holds the calls the group refuses on a *testing.T.
func TestProbe(t *testing.T) {
	t.Setenv("SHIFT2EU_TOKEN", "example") // want "tests-no-process-state: "
	t.Chdir("example")                    // want "tests-no-process-state: "

	t.TempDir()
}
