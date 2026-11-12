//go:build windows

// The windows entry points of syscall, for TestCompositionRootForbidigoProbe's
// windows run: the release builds do. Every line that ends in a want comment
// must be refused with a msg of the group it names, as in main.go. The file
// builds for windows only, which is where its names type-check, and the probe
// reads its want comments in that run alone.
package main

import (
	"os"
	"syscall"
)

func main() {}

// windowsProcess starts a program, or loads the library that would run its code:
// a composition root starts nothing here either.
func windowsProcess() {
	_ = syscall.CreateProcess(nil, nil, nil, nil, false, 0, nil, nil, nil, nil)          // want "composition-root-no-exec: "
	_ = syscall.CreateProcessAsUser(0, nil, nil, nil, nil, false, 0, nil, nil, nil, nil) // want "composition-root-no-exec: "
	_, _ = syscall.GetEnvironmentStrings()                                               // want "composition-root-no-exec: "
	_, _ = syscall.LoadDLL("kernel32.dll")                                               // want "composition-root-no-exec: "
	_ = syscall.MustLoadDLL("kernel32.dll")                                              // want "composition-root-no-exec: "
	_ = syscall.NewLazyDLL("kernel32.dll")                                               // want "composition-root-no-exec: "
	library, _ := syscall.LoadLibrary("kernel32.dll")                                    // want "composition-root-no-exec: "
	_, _ = syscall.GetProcAddress(library, "GetEnvironmentStringsW")                     // want "composition-root-no-exec: "
	_, _, _ = syscall.SyscallN(0)                                                        // want "composition-root-no-exec: "
	_, _, _ = syscall.Syscall9(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)                          // want "composition-root-no-exec: "
}

// windowsVariables reads one named variable and dumps the whole environment,
// which the two groups separate as on unix.
func windowsVariables() {
	_, _ = syscall.GetEnvironmentVariable(nil, nil, 0) // want "composition-root-env: "
	_ = os.Getenv("SHIFT2EU_TOKEN")                    // want "composition-root-env: "
	_, _ = os.LookupEnv("SHIFT2EU_TOKEN")              // want "composition-root-env: "
	_ = os.ExpandEnv("$SHIFT2EU_TOKEN")                // want "composition-root-env: "
	_, _ = syscall.Getenv("SHIFT2EU_TOKEN")            // want "composition-root-env: "
}
