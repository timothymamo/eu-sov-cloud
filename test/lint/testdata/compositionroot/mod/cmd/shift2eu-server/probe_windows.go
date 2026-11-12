//go:build windows

// The windows entry points of syscall for the server composition root, for
// TestCompositionRootForbidigoProbe's windows run. It holds the calls main.go
// and server.go hold for the host's platform, so the groups' windows entries
// fire here, and it repeats what the env group refuses beside them. The file
// builds for windows only, and the probe reads its want comments in that run.
package main

import (
	"os"
	"syscall"
)

func main() {}

// windowsProcess starts a program, or loads the library that would run its code.
func windowsProcess() {
	_ = syscall.CreateProcess(nil, nil, nil, nil, false, 0, nil, nil, nil, nil)          // want "composition-root-no-exec: "
	_ = syscall.CreateProcessAsUser(0, nil, nil, nil, nil, false, 0, nil, nil, nil, nil) // want "composition-root-no-exec: "
	_, _ = syscall.GetEnvironmentStrings()                                               // want "composition-root-no-exec: "
	_, _ = syscall.LoadDLL("kernel32.dll")                                               // want "composition-root-no-exec: "
	_ = syscall.MustLoadDLL("kernel32.dll")                                              // want "composition-root-no-exec: "
	_ = syscall.NewLazyDLL("kernel32.dll")                                               // want "composition-root-no-exec: "
	library, _ := syscall.LoadLibrary("kernel32.dll")                                    // want "composition-root-no-exec: "
	_, _ = syscall.GetProcAddress(library, "CreateProcessW")                             // want "composition-root-no-exec: "
	_, _, _ = syscall.SyscallN(0)                                                        // want "composition-root-no-exec: "
}

// windowsVariables reads one named variable, and dumps the whole environment
// with the windows name of the same call.
func windowsVariables() {
	_, _ = syscall.GetEnvironmentVariable(nil, nil, 0) // want "composition-root-env: "
	_ = os.Getenv("SHIFT2EU_SERVER_TOKEN")             // want "composition-root-env: "
	_, _ = os.LookupEnv("SHIFT2EU_SERVER_TOKEN")       // want "composition-root-env: "
	_ = os.ExpandEnv("$SHIFT2EU_SERVER_TOKEN")         // want "composition-root-env: "
	_, _ = syscall.Getenv("SHIFT2EU_SERVER_TOKEN")     // want "composition-root-env: "
}
