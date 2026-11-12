// The test helpers' _test.go file for TestTestsNoProcessStateProbe (issue #153):
// the group tests-no-process-state holds for internal/testutil as it holds for
// the application layer's tests, because the helpers are compiled into every test
// that imports them and a helper's test runs in the same process. Every line
// below ends in a want comment of the group: t.Setenv and t.Chdir change the
// environment and the working directory of the whole test binary, which a helper
// may not do for the tests that use it (CLAUDE.md rule 3: the environment is
// where the credentials live).
//
// t.TempDir keeps no want comment: a helper that needs a directory takes a fresh
// one, not a home, config or credential path. Nothing here runs: the probe lints
// the file.
package testutil_test

import "testing"

// TestProbe holds the calls the group refuses on a *testing.T, and the ones it
// must keep allowing.
func TestProbe(t *testing.T) {
	t.Setenv("SHIFT2EU_TOKEN", "example") // want "tests-no-process-state: "
	t.Chdir("example")                    // want "tests-no-process-state: "

	t.TempDir()
	t.Parallel()
	t.Helper()
}

// viaTB holds the calls a helper makes through the testing.TB interface, which a
// test passes as *testing.T, *testing.B or *testing.F.
func viaTB(tb testing.TB) {
	tb.Setenv("SHIFT2EU_TOKEN", "example") // want "tests-no-process-state: "
	tb.Chdir("example")                    // want "tests-no-process-state: "
	tb.TempDir()
}
