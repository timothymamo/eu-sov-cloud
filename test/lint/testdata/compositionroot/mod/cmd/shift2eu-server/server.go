//go:build !windows

// Package main is the second file of the server composition root, for
// TestCompositionRootForbidigoProbe. The os.LookupEnv exception holds for
// cmd/shift2eu-server/main.go alone, so the same reference is refused here. The
// file is for the host's platform: the windows entry points of syscall are in
// probe_windows.go, which the probe's windows run lints.
package main

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

// process starts a process or replaces this one with it.
func process() {
	_, _ = os.StartProcess("/bin/true", nil, nil)              // want "composition-root-no-exec: "
	_ = syscall.Exec("/bin/true", nil, nil)                    // want "composition-root-no-exec: "
	_, _ = syscall.ForkExec("/bin/true", nil, nil)             // want "composition-root-no-exec: "
	_, _, _ = syscall.StartProcess("/bin/true", nil, nil)      // want "composition-root-no-exec: "
	_ = exec.Command("/bin/true")                              // want "composition-root-no-exec: "
	_ = exec.CommandContext(context.Background(), "/bin/true") // want "composition-root-no-exec: "
	_, _ = exec.LookPath("true")                               // want "composition-root-no-exec: "
	_ = os.Environ()                                           // want "composition-root-no-exec: "
	_ = syscall.Environ()                                      // want "composition-root-no-exec: "
}

// syscalls makes the kernel call the process entry points are built on, and
// names the number of the exec call: the composition roots may not do it this
// way either.
func syscalls() {
	_, _, _ = syscall.Syscall(0, 0, 0, 0)              // want "composition-root-no-exec: "
	_, _, _ = syscall.Syscall6(0, 0, 0, 0, 0, 0, 0)    // want "composition-root-no-exec: "
	_, _, _ = syscall.RawSyscall(0, 0, 0, 0)           // want "composition-root-no-exec: "
	_, _, _ = syscall.RawSyscall6(0, 0, 0, 0, 0, 0, 0) // want "composition-root-no-exec: "
	_ = syscall.SYS_EXECVE                             // want "composition-root-no-exec: "
}

// variables reads one named variable, os.LookupEnv included: the exception is
// main.go of this command, not this file.
func variables() {
	_ = os.Getenv("SHIFT2EU_SERVER_TOKEN")         // want "composition-root-env: "
	_, _ = os.LookupEnv("SHIFT2EU_SERVER_TOKEN")   // want "composition-root-env: "
	_, _ = syscall.Getenv("SHIFT2EU_SERVER_TOKEN") // want "composition-root-env: "
	_ = os.ExpandEnv("$SHIFT2EU_SERVER_TOKEN")     // want "composition-root-env: "
}
