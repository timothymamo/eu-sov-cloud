// The e2e tier's stand-in for TestE2ENoEnvProbe (issue #167). depguard's
// e2e-is-black-box allows $gostd, every standard package, os and syscall among
// them, and both read the environment, where credentials live (CLAUDE.md rule
// 3), start a process with it, or locate a home, config, cache or temp directory
// from an unnamed variable. The forbidigo group e2e-no-env refuses those calls.
//
// Every line below that ends in a want comment is a call the group must refuse;
// the calls the e2e tests make (os/exec, a signal, a file read) keep no want
// comment, so a group that stops firing, or one that reaches them, fails the
// probe. Only names every unix platform has are called, so the probe builds on
// the host. Nothing here runs: the probe lints the file.
package e2e

import (
	"os"
	"os/exec"
	"syscall"
)

// osReads makes every call of os that reaches the environment or a directory an
// environment variable names.
func osReads() {
	_ = os.Getenv("AWS_PROFILE")                  // want "e2e-no-env: "
	_, _ = os.LookupEnv("AWS_PROFILE")            // want "e2e-no-env: "
	_ = os.ExpandEnv("$AWS_PROFILE")              // want "e2e-no-env: "
	_ = os.Environ()                              // want "e2e-no-env: "
	_, _ = os.StartProcess("/bin/true", nil, nil) // want "e2e-no-env: "
	_, _ = os.UserHomeDir()                       // want "e2e-no-env: "
	_, _ = os.UserConfigDir()                     // want "e2e-no-env: "
	_, _ = os.UserCacheDir()                      // want "e2e-no-env: "
	_ = os.TempDir()                              // want "e2e-no-env: "
}

// syscallReads makes every call of syscall that reaches the environment, starts
// or replaces a process, or makes a raw system call.
func syscallReads() {
	_, _ = syscall.Getenv("AWS_PROFILE")                  // want "e2e-no-env: "
	_ = syscall.Environ()                                 // want "e2e-no-env: "
	_ = syscall.Exec("/bin/true", nil, nil)               // want "e2e-no-env: "
	_, _ = syscall.ForkExec("/bin/true", nil, nil)        // want "e2e-no-env: "
	_, _, _ = syscall.StartProcess("/bin/true", nil, nil) // want "e2e-no-env: "
	_, _, _ = syscall.Syscall(0, 0, 0, 0)                 // want "e2e-no-env: "
	_, _, _ = syscall.Syscall6(0, 0, 0, 0, 0, 0, 0)       // want "e2e-no-env: "
	_, _, _ = syscall.RawSyscall(0, 0, 0, 0)              // want "e2e-no-env: "
	_, _, _ = syscall.RawSyscall6(0, 0, 0, 0, 0, 0, 0)    // want "e2e-no-env: "
}

// runs builds and runs the binary as the e2e tests do: the group keeps these
// allowed.
func runs(path string) {
	cmd := exec.Command(path, "assess")
	if err := cmd.Start(); err != nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	_ = cmd.Wait()

	_, _ = os.ReadFile(path)
}
