// Command shift2eu is one of the two composition roots, for
// TestCompositionRootForbidigoProbe. Every line that ends in a want comment
// holds a call that one of the two composition-root groups refuses, and the
// comment names the group's tag, which starts the msg of every entry in it:
// composition-root-no-exec for a process and the whole environment, and
// composition-root-env for one named variable. There is a line for every
// function the design forbids, here and in cmd/shift2eu-server, and the same
// calls in internal/cli must pass, as the groups hold for the composition
// roots only.
package main

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

func main() {}

// start starts a process or replaces this one with it, which would run a
// program the operator of a report did not ask for.
func start() {
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

// environment reads one named variable, which the composition-root-env group
// refuses everywhere but cmd/shift2eu-server/main.go.
func environment() {
	_ = os.Getenv("SHIFT2EU_TOKEN")         // want "composition-root-env: "
	_, _ = os.LookupEnv("SHIFT2EU_TOKEN")   // want "composition-root-env: "
	_, _ = syscall.Getenv("SHIFT2EU_TOKEN") // want "composition-root-env: "
	_ = os.ExpandEnv("$SHIFT2EU_TOKEN")     // want "composition-root-env: "
}
