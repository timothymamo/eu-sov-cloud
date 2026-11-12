// The helper's temporary-directory stand-in: os.MkdirTemp("", ...) and
// os.CreateTemp("", ...) read $TMPDIR implicitly, through os.TempDir, and
// TestTestsNoProcessStateProbe holds that the group tests-no-process-state
// allows them (issue #153). They return a directory and a file of their own, made
// fresh and removed with the test, never a home, config or credential path, so
// refusing them would leave a test no way to get a temporary directory that
// t.TempDir itself does not also lose (it reads $TMPDIR the same way).
// os.TempDir named directly stays refused, where testutil-no-env refuses it.
//
// Neither line carries a want comment, so a group that refuses a temp-directory
// helper fails the probe. Nothing here runs: the probe lints the file.
package testutil

import "os"

// TempDir returns a fresh temporary directory of its own, which the test removes
// with the test binary.
func TempDir() (string, error) {
	return os.MkdirTemp("", "shift2eu-test-")
}

// TempFile returns a fresh temporary file of its own.
func TempFile() (*os.File, error) {
	return os.CreateTemp("", "shift2eu-test-")
}
