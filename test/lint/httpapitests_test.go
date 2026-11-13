package lint_test

import (
	"slices"
	"strings"
	"testing"
)

// httpapiTestsNoEnvOrProcessTag is the tag of forbidigo's
// httpapi-tests-no-env-or-process group (issue #152): the calls the HTTP API's
// _test.go files may not make, which reach the environment, locate a directory
// from an unnamed variable or start a process. A composition root may not make
// them itself (composition-root-env, composition-root-no-exec), and neither may
// the CLI (cli-no-env-or-process), but the roots' groups stop at cmd/ and the
// CLI's stops at internal/cli/, so before this group the handler's tests could
// reach the environment the process and the code under test share, where
// credentials live (CLAUDE.md rule 3), with lint green. The tests of
// internal/cli are held by that package's own group, which this one does not
// repeat: no line is reported by two groups.
//
// The group refuses every way to read a named variable (os.Getenv,
// os.LookupEnv, os.ExpandEnv), to dump the whole environment (os.Environ), to
// start a process (os.StartProcess) and the four lookups that locate a
// directory from an unnamed variable (os.UserHomeDir, os.UserConfigDir,
// os.UserCacheDir, os.TempDir), exactly as cli-no-env-or-process does, and the
// syscall calls the composition roots' groups refuse (syscall.Getenv,
// GetEnvironmentVariable, Environ, Exec, ForkExec, StartProcess, the windows
// entry points and the raw system calls, syscall.SYS_* among them). A test may
// import package syscall: internal/cli's output_unix_test.go takes
// syscall.Mkfifo, and no group may refuse that.
//
// os.Setenv, os.Unsetenv, os.Clearenv and os.Chdir are no entry of this group:
// the group tests-no-process-state refuses them in every test of the module
// (issue #153). It holds for the _test.go files under internal/httpapi/ only,
// which its linters.exclusions.rules say and
// TestForbidigoExclusionsDropOneGroup checks.
const httpapiTestsNoEnvOrProcessTag = "httpapi-tests-no-env-or-process"

// httpapiTestsNoEnvOrProcessRefused lists every name the group must refuse. A
// name is refused when an entry of the group matches its pkg and its rendered
// name; the wording of a msg is not pinned here, the tag is.
var httpapiTestsNoEnvOrProcessRefused = []rootName{
	{"os", "os.Getenv"},
	{"os", "os.LookupEnv"},
	{"os", "os.ExpandEnv"},
	{"os", "os.Environ"},
	{"os", "os.StartProcess"},
	{"os", "os.UserHomeDir"},
	{"os", "os.UserConfigDir"},
	{"os", "os.UserCacheDir"},
	{"os", "os.TempDir"},
	{"syscall", "syscall.Getenv"},
	{"syscall", "syscall.GetEnvironmentVariable"},
	{"syscall", "syscall.Environ"},
	{"syscall", "syscall.Exec"},
	{"syscall", "syscall.ForkExec"},
	{"syscall", "syscall.StartProcess"},
	{"syscall", "syscall.CreateProcess"},
	{"syscall", "syscall.CreateProcessAsUser"},
	{"syscall", "syscall.GetEnvironmentStrings"},
	{"syscall", "syscall.LoadDLL"},
	{"syscall", "syscall.MustLoadDLL"},
	{"syscall", "syscall.NewLazyDLL"},
	{"syscall", "syscall.LoadLibrary"},
	{"syscall", "syscall.GetProcAddress"},
	{"syscall", "syscall.Syscall"},
	{"syscall", "syscall.Syscall6"},
	{"syscall", "syscall.Syscall9"},
	{"syscall", "syscall.Syscall12"},
	{"syscall", "syscall.Syscall15"},
	{"syscall", "syscall.Syscall18"},
	{"syscall", "syscall.SyscallN"},
	{"syscall", "syscall.RawSyscall"},
	{"syscall", "syscall.RawSyscall6"},
	{"syscall", "syscall.AllThreadsSyscall"},
	{"syscall", "syscall.AllThreadsSyscall6"},
	{"syscall", "syscall.SYS_EXECVE"},
	{"syscall", "syscall.SYS_CLONE"},
}

// httpapiTestsNoEnvOrProcessAllowed are names no entry of the group may refuse:
// what a test of the handler is for (the temporary directory it takes, the
// fixture files it names), syscall.Mkfifo, which is why a test imports package
// syscall at all, the process names that are no call on a variable, the names
// that only extend a refused one (os.Getenvs, os.Environments, os.StartProcesses,
// os.UserHomeDirs, syscall.Getenvs, syscall.Environments, syscall.Execve,
// syscall.Syscalls, syscall.RawSyscalls, syscall.SYSCTL) and catch a pattern
// that lost its $, and a package named os or syscall elsewhere, which is no file
// of this module's.
var httpapiTestsNoEnvOrProcessAllowed = []rootName{
	{"os", "os.Open"},
	{"os", "os.OpenFile"},
	{"os", "os.ReadFile"},
	{"os", "os.Create"},
	{"os", "os.CreateTemp"},
	{"os", "os.MkdirTemp"},
	{"os", "os.Exit"},
	{"os", "os.Args"},
	{"os", "os.Getpid"},
	{"os", "os.Getenvs"},
	{"os", "os.Environments"},
	{"os", "os.StartProcesses"},
	{"os", "os.UserHomeDirs"},
	{"syscall", "syscall.Mkfifo"},
	{"syscall", "syscall.SIGTERM"},
	{"syscall", "syscall.Signal"},
	{"syscall", "syscall.Getpid"},
	{"syscall", "syscall.Getenvs"},
	{"syscall", "syscall.Environments"},
	{"syscall", "syscall.Execve"},
	{"syscall", "syscall.Syscalls"},
	{"syscall", "syscall.RawSyscalls"},
	{"syscall", "syscall.SYSCTL"},
	{"example.com/otheros", "os.Getenv"},
	{"example.com/othersyscall", "syscall.Getenv"},
}

// TestHTTPAPITestsNoEnvOrProcessForbidigoPatterns checks the entries of the
// httpapi-tests-no-env-or-process group, read from .golangci.yml, offline and
// without golangci-lint: every name the HTTP API's tests may not use is refused
// by an entry of the group, no name they may use is refused by one, and every
// entry refuses a listed name. The probe (TestCompositionRootForbidigoProbe)
// proves how forbidigo renders a name; this pins the patterns and their
// packages over more names than a probe can hold, the windows entry points
// among them, which no host probe can type-check.
func TestHTTPAPITestsNoEnvOrProcessForbidigoPatterns(t *testing.T) {
	t.Parallel()

	entries := groupEntries(t, moduleRoot(t), httpapiTestsNoEnvOrProcessTag)
	refuses := func(e forbidEntry, n rootName) bool {
		return e.pkg.MatchString(n.pkg) && e.pattern.MatchString(n.name)
	}

	for _, r := range httpapiTestsNoEnvOrProcessRefused {
		var msgs []string

		for _, e := range entries {
			if refuses(e, r) {
				msgs = append(msgs, e.msg)
			}
		}

		if !slices.ContainsFunc(msgs, func(m string) bool { return strings.HasPrefix(m, httpapiTestsNoEnvOrProcessTag+": ") }) {
			t.Errorf("%s (pkg %q) must be refused by an entry of group %s; refused by %q", r.name, r.pkg, httpapiTestsNoEnvOrProcessTag, msgs)
		}
	}

	for _, n := range httpapiTestsNoEnvOrProcessAllowed {
		for _, e := range entries {
			if refuses(e, n) {
				t.Errorf("%s (pkg %q) must pass a test, but the entry with msg %q refuses it", n.name, n.pkg, e.msg)
			}
		}
	}

	for _, e := range entries {
		if !slices.ContainsFunc(httpapiTestsNoEnvOrProcessRefused, func(r rootName) bool { return refuses(e, r) }) {
			t.Errorf("the entry with msg %q refuses no name of the group's list; add the names it refuses", e.msg)
		}
	}
}
