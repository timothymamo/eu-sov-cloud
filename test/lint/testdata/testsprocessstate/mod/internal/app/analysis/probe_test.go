// The application layer's _test.go file for TestTestsNoProcessStateProbe (issue
// #153): the group tests-no-process-state refuses t.Setenv and t.Chdir on a
// test's *testing.T, *testing.B, *testing.F and on the testing.TB interface, and
// every line below ends in a want comment of that group. A test that sets an
// environment variable or changes the working directory does so for the whole
// process: the test binary runs its tests in one process, t.Parallel lets them
// run side by side, and a variable another test reads — a credential's name
// among them (CLAUDE.md rule 3) — is what the other test then sees. The case it
// wants is the one it passes to the code under test, through a fake or a
// parameter, and a directory it needs it takes from t.TempDir.
//
// t.TempDir keeps no want comment, and must not be refused: it reads $TMPDIR
// implicitly (through os.TempDir) and returns a fresh directory of its own, never
// a home, config or credential path. os.MkdirTemp("", ...) and
// os.CreateTemp("", ...) are allowed for the same reason, which
// internal/testutil's temp.go holds; os.TempDir named directly stays refused
// where testutil-no-env refuses it today.
//
// Nothing here runs: the probe lints the file.
package analysis_test

import "testing"

// TestProbe holds one call for each way a test can change the process state, for
// every receiver the group must refuse.
func TestProbe(t *testing.T) {
	t.Setenv("SHIFT2EU_TOKEN", "example") // want "tests-no-process-state: "
	t.Chdir("example")                    // want "tests-no-process-state: "
	// A method value is no call, and the group must refuse it too: taken
	// alone, it hands the write to whoever the test passes it to.
	_ = t.Setenv // want "tests-no-process-state: "

	// t.TempDir stays allowed, and so does everything else testing hands a
	// test.
	t.TempDir()
	t.Parallel()
	t.Helper()
	t.Log("probe")
	t.Cleanup(func() {})
	t.Run("sub", func(t *testing.T) {})
}

// BenchmarkProbe holds the calls on a benchmark's *testing.B, which is the same
// process.
func BenchmarkProbe(b *testing.B) {
	b.Setenv("SHIFT2EU_TOKEN", "example") // want "tests-no-process-state: "
	b.Chdir("example")                    // want "tests-no-process-state: "
	b.TempDir()
}

// FuzzProbe holds the calls on a fuzz target's *testing.F.
func FuzzProbe(f *testing.F) {
	f.Setenv("SHIFT2EU_TOKEN", "example") // want "tests-no-process-state: "
	f.Chdir("example")                    // want "tests-no-process-state: "
	f.TempDir()
}

// viaTB holds the calls a helper makes through the testing.TB interface, which a
// test passes as *testing.T, *testing.B or *testing.F: a group that names the
// three concrete types alone leaves this way round it open.
func viaTB(tb testing.TB) {
	tb.Setenv("SHIFT2EU_TOKEN", "example") // want "tests-no-process-state: "
	tb.Chdir("example")                    // want "tests-no-process-state: "
	tb.TempDir()
}
