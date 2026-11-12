//go:build !windows

// The HTTP API's _test.go file for TestCompositionRootForbidigoProbe (issue
// #152): a test of the handler runs in the same process as the handler and as
// the composition root that hands it the work, so the group
// httpapi-tests-no-env-or-process holds it to the calls the composition roots'
// tests may not make either. Before #152 no rule of this kind reached it: the
// roots' groups stop at cmd/, cli-no-env-or-process stops at internal/cli/, and
// the composition-root-tests depguard rule refuses only an import, so a test
// could read the environment, dump it, locate a home or config directory, start
// a process or make a raw system call with lint green.
//
// Every line below that ends in a want comment is one of those calls, and the
// want names the group's tag. os.Setenv, os.Unsetenv, os.Clearenv and os.Chdir
// are not here: the group tests-no-process-state refuses them in every test of
// the module (issue #153), and no line is reported by two groups. The last two
// functions are what a test of the handler keeps: syscall.Mkfifo (a test may
// import package syscall), the temporary directory it takes, and the fixture
// files it names. None carries a want, so a group that refuses package syscall
// or os as a whole, or a pattern that lost its $, fails the probe.
package httpapi_test

import (
	"os"
	"syscall"
	"testing"
)

// TestEnvironmentReads holds the calls of os and syscall that read or hand over
// the environment, locate a directory from an unnamed variable and start a
// process, in a test rather than in the handler.
func TestEnvironmentReads(t *testing.T) {
	_ = os.Getenv("SHIFT2EU_TOKEN")               // want "httpapi-tests-no-env-or-process: "
	_, _ = os.LookupEnv("SHIFT2EU_TOKEN")         // want "httpapi-tests-no-env-or-process: "
	_ = os.ExpandEnv("$SHIFT2EU_TOKEN")           // want "httpapi-tests-no-env-or-process: "
	_ = os.Environ()                              // want "httpapi-tests-no-env-or-process: "
	_, _ = os.StartProcess("/bin/true", nil, nil) // want "httpapi-tests-no-env-or-process: "
	_, _ = os.UserHomeDir()                       // want "httpapi-tests-no-env-or-process: "
	_, _ = os.UserConfigDir()                     // want "httpapi-tests-no-env-or-process: "
	_, _ = os.UserCacheDir()                      // want "httpapi-tests-no-env-or-process: "
	_ = os.TempDir()                              // want "httpapi-tests-no-env-or-process: "

	_, _ = syscall.Getenv("SHIFT2EU_TOKEN")        // want "httpapi-tests-no-env-or-process: "
	_ = syscall.Environ()                          // want "httpapi-tests-no-env-or-process: "
	_ = syscall.Exec("/bin/true", nil, nil)        // want "httpapi-tests-no-env-or-process: "
	_, _ = syscall.ForkExec("/bin/true", nil, nil) // want "httpapi-tests-no-env-or-process: "
	_, _, _ = syscall.Syscall(0, 0, 0, 0)          // want "httpapi-tests-no-env-or-process: "
	_, _, _ = syscall.RawSyscall(0, 0, 0, 0)       // want "httpapi-tests-no-env-or-process: "
	_ = syscall.SYS_EXECVE                         // want "httpapi-tests-no-env-or-process: "

	// A method value is no call, and the group must refuse it too: taken
	// alone, it hands the read to whoever the test passes it to.
	_ = os.Getenv // want "httpapi-tests-no-env-or-process: "

	t.TempDir()
}

// TestFiles holds what a test of the handler does with package syscall and os,
// and must keep doing: a FIFO, as internal/cli's test makes for the report, the
// temporary directory it takes, and the request documents it names.
func TestFiles(t *testing.T) {
	path := t.TempDir() + "/request.json"

	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}

	_, _ = os.ReadFile(path)
	_, _ = os.Create(path)
	_, _ = os.CreateTemp("", "shift2eu-httpapi-*.json")
	_ = os.Getpid()
}
