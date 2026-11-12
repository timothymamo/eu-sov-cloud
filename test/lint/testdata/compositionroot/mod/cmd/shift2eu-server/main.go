//go:build !windows

// Command shift2eu-server is the other composition root, for
// TestCompositionRootForbidigoProbe. It hands os.LookupEnv to run for
// SHIFT2EU_SERVER_TOKEN (ADR 0019), and the composition-root-env group allows
// exactly that one function in exactly this one file: the line below carries no
// want comment. Every other call here is refused, so the exception is that one
// function in this file, not the whole group in it, and the exclusion rules are
// pinned by TestCompositionRootExclusions. The file is for the host's platform:
// the windows entry points of syscall are in probe_windows.go, which the probe's
// windows run lints.
package main

import (
	"os"
	"syscall"
)

// run stands in for the server's run, which takes the environment lookup as a
// parameter rather than reading the environment itself.
func run(lookup func(string) (string, bool)) {}

func main() {
	run(os.LookupEnv) // the one exception: no composition-root-env issue here

	_ = os.Getenv("SHIFT2EU_SERVER_TOKEN")         // want "composition-root-env: "
	_, _ = syscall.Getenv("SHIFT2EU_SERVER_TOKEN") // want "composition-root-env: "
	_ = os.ExpandEnv("$SHIFT2EU_SERVER_TOKEN")     // want "composition-root-env: "

	_ = os.Environ()                              // want "composition-root-no-exec: "
	_, _ = os.StartProcess("/bin/true", nil, nil) // want "composition-root-no-exec: "
	_, _, _ = syscall.Syscall(0, 0, 0, 0)         // want "composition-root-no-exec: "
	_ = syscall.SYS_EXECVE                        // want "composition-root-no-exec: "
}
