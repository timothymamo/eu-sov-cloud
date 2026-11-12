//go:build !windows

// The CLI's _test.go file for TestCompositionRootForbidigoProbe (issue #152):
// a test of the CLI runs in the same process as the code under test and as the
// composition root that hands it the work, so it is held to the calls the CLI's
// production code may not make either. Before #152 only the production files
// were held: cli-gets-adapters-from-cmd skipped _test.go files, and the group
// cli-no-env-or-process said nothing of package syscall, which a test may import
// (internal/cli/output_unix_test.go takes syscall.Mkfifo). So a test could read
// the environment, dump it, start a process or make a raw system call with lint
// green.
//
// Every line below that ends in a want comment is one of those calls, and the
// want names the group's tag. The aliased import is the same package under
// another name (xos.Getenv is os.Getenv), so the probe pins that the group
// follows the import's path rather than the name a file gives it. The last two
// functions are what the tests keep: syscall.Mkfifo, which makes the FIFO
// output_unix_test.go needs, and the calls of os that read and write the files
// a test names. Neither carries a want, so a group that refuses package syscall
// or os as a whole, or a pattern that lost its $, fails the probe.
package cli_test

import (
	"os"
	xos "os"
	"syscall"
	"testing"
)

// TestEnvironmentReads holds the calls of os and syscall that read or hand over
// the environment and start a process, in a test rather than in the CLI.
func TestEnvironmentReads(t *testing.T) {
	_ = os.Getenv("SHIFT2EU_TOKEN")               // want "cli-no-env-or-process: "
	_, _ = os.LookupEnv("SHIFT2EU_TOKEN")         // want "cli-no-env-or-process: "
	_ = os.Environ()                              // want "cli-no-env-or-process: "
	_ = xos.Getenv("SHIFT2EU_TOKEN")              // want "cli-no-env-or-process: "
	_, _ = os.UserHomeDir()                       // want "cli-no-env-or-process: "
	_, _ = os.StartProcess("/bin/true", nil, nil) // want "cli-no-env-or-process: "

	_, _ = syscall.Getenv("SHIFT2EU_TOKEN")        // want "cli-no-env-or-process: "
	_ = syscall.Environ()                          // want "cli-no-env-or-process: "
	_ = syscall.Exec("/bin/true", nil, nil)        // want "cli-no-env-or-process: "
	_, _ = syscall.ForkExec("/bin/true", nil, nil) // want "cli-no-env-or-process: "
	_, _, _ = syscall.Syscall(0, 0, 0, 0)          // want "cli-no-env-or-process: "
	_, _, _ = syscall.RawSyscall(0, 0, 0, 0)       // want "cli-no-env-or-process: "
	_ = syscall.SYS_EXECVE                         // want "cli-no-env-or-process: "

	t.TempDir()
}

// TestFiles holds what the CLI's tests do with package syscall and os, and must
// keep doing: a FIFO to write the report to, and the fixture files a test names.
func TestFiles(t *testing.T) {
	path := t.TempDir() + "/inventory.json"

	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}

	_, _ = os.ReadFile(path)
	_, _ = os.Create(path)
	_ = os.Getpid()
}
