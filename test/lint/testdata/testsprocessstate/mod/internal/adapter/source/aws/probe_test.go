// A cloud source adapter's _test.go file for TestTestsNoProcessStateProbe (issue
// #153, the review's finding S2): the group tests-no-process-state holds for every
// test of the module but the two harnesses under test/, so an adapter's test is no
// exception. The likely case is the one below: a test that sets AWS_PROFILE, or the
// region variable an SDK reads, changes the environment of the binary every other
// adapter's test runs in, and a credential's name among them (CLAUDE.md rule 3).
// A test hands the profile to the code under test as a parameter, or through a
// fake SDK client.
//
// t.TempDir, os.MkdirTemp("", ...) and os.CreateTemp("", ...) keep no want
// comment: they read $TMPDIR implicitly and return a fresh temporary path, so a
// test that needs a directory takes one. Nothing here runs: the probe lints the
// file.
package aws_test

import (
	"os"
	"syscall"
	"testing"
)

// TestProbe holds the calls the group refuses on a *testing.T, and the temporary
// directory it allows.
func TestProbe(t *testing.T) {
	t.Setenv("AWS_PROFILE", "example") // want "tests-no-process-state: "
	t.Chdir("example")                 // want "tests-no-process-state: "

	t.TempDir()
	_, _ = os.MkdirTemp("", "shift2eu-aws-*")
	_, _ = os.CreateTemp("", "shift2eu-aws-*")
}

// TestEnvironmentWrites holds the calls of os that write the environment or the
// working directory of the process, one package further out than t.Setenv and
// t.Chdir (the review's finding S1).
func TestEnvironmentWrites(t *testing.T) {
	_ = os.Setenv("AWS_PROFILE", "example") // want "tests-no-process-state: "
	_ = os.Unsetenv("AWS_PROFILE")          // want "tests-no-process-state: "
	os.Clearenv()                           // want "tests-no-process-state: "
	_ = os.Chdir("example")                 // want "tests-no-process-state: "

	t.TempDir()
}

// TestSyscallEnvironmentWrites holds the calls of syscall that write the
// environment or change the working directory under names of their own, which
// the group's second entry refuses (the round-2 review's finding S1: syscall
// names neither Setenv nor Chdir, so the receiver-agnostic entry misses them).
func TestSyscallEnvironmentWrites(t *testing.T) {
	_ = syscall.Unsetenv("AWS_PROFILE") // want "tests-no-process-state: "
	syscall.Clearenv()                  // want "tests-no-process-state: "
	_ = syscall.Fchdir(3)               // want "tests-no-process-state: "

	t.TempDir()
}
