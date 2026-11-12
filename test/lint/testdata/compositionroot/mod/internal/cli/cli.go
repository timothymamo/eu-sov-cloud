// Package cli is neither composition root, so the two composition-root groups
// do not hold for it: nothing below is reported, and a group that lost its
// path-except scope is refused by the lines that start a process here.
package cli

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

// Start starts a process or replaces this one with it, which the composition
// roots must not do.
func Start() {
	_, _ = os.StartProcess("/bin/true", nil, nil)
	_ = syscall.Exec("/bin/true", nil, nil)
	_, _ = syscall.ForkExec("/bin/true", nil, nil)
	_, _, _ = syscall.StartProcess("/bin/true", nil, nil)
	_ = exec.Command("/bin/true")
	_ = exec.CommandContext(context.Background(), "/bin/true")
	_, _ = exec.LookPath("true")
	_ = os.Environ()
	_ = syscall.Environ()
}

// Environment reads one named variable, which is the CLI's caller's job.
func Environment() {
	_ = os.Getenv("SHIFT2EU_TOKEN")
	_, _ = os.LookupEnv("SHIFT2EU_TOKEN")
	_, _ = syscall.Getenv("SHIFT2EU_TOKEN")
	_ = os.ExpandEnv("$SHIFT2EU_TOKEN")
}
