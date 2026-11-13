package lint_test

import (
	"slices"
	"strings"
	"testing"
)

// cliNoEnvOrProcessTag is the tag of forbidigo's cli-no-env-or-process group
// (issue #152): the calls the CLI's production code may not make, which reach
// the environment or start a process. A composition root reads its one named
// variable and passes the rest to the CLI, and the composition-root-env group
// stops at cmd/: before this group, internal/cli's import rule allowed $gostd,
// so a root could hand a read it may not make itself to the CLI and the read
// happened one call deeper, with lint green. CLAUDE.md rules 1 and 3 are why it
// may not: the environment is where credentials live.
//
// The group refuses every way to read a named variable (os.Getenv, os.LookupEnv,
// os.ExpandEnv), to dump the whole environment (os.Environ) and to start a
// process (os.StartProcess), and the four lookups that locate a directory from
// an unnamed variable (os.UserHomeDir, os.UserConfigDir, os.UserCacheDir,
// os.TempDir): a CLI that builds the path to ~/.config/dot-agent-deck/daemon.env
// or ~/.aws/credentials names no variable while it reads one. The writes of one
// (os.Setenv, os.Unsetenv, os.Clearenv) and os.Chdir are the group
// tests-no-process-state's since issue #153, which refuses them in every file of
// the module but the two harnesses under test/, so one group owns them and no
// line is reported twice. It refuses the
// syscall calls the composition roots' groups refuse as well (syscall.Getenv,
// GetEnvironmentVariable, Environ, Exec, ForkExec, StartProcess, the windows
// entry points, the raw system calls and syscall.SYS_*), which reach the same
// environment and start the same processes one package lower. It holds for files
// under internal/cli/, _test.go files included, which its linters.exclusions.rules
// say and TestForbidigoExclusionsDropOneGroup checks. The syscall entries are
// what holds the CLI's _test.go files (issue #152): cli-gets-adapters-from-cmd
// refuses package syscall to the production files, yet a test imports it for
// syscall.Mkfifo (internal/cli/output_unix_test.go), which the group must keep
// allowed.
//
// unsafe and os/exec are refused at the import by the CLI's strict allow list
// (cli-gets-adapters-from-cmd, issue #152), which TestCLIImportsProbe lints, so
// no call of theirs needs an entry here. The syscall entries are no substitute
// for that rule: package syscall stays refused in the CLI's production files, and
// the entries are the call-level rule for its tests. The calls the CLI is for —
// os.Open, os.ReadFile, os.Create and os.CreateTemp, the inventory a user names
// and the report the user asked for — are no calls of this group.
const cliNoEnvOrProcessTag = "cli-no-env-or-process"

// cliNoEnvOrProcessRefused lists every name the group must refuse. A name is
// refused when an entry of the group matches its pkg and its rendered name; the
// wording of a msg is not pinned here, the tag is.
var cliNoEnvOrProcessRefused = []rootName{
	{"os", "os.Getenv"},
	{"os", "os.LookupEnv"},
	{"os", "os.ExpandEnv"},
	{"os", "os.Environ"},
	{"os", "os.StartProcess"},
	{"os", "os.UserHomeDir"},
	{"os", "os.UserConfigDir"},
	{"os", "os.UserCacheDir"},
	{"os", "os.TempDir"},
	// The syscall calls: the CLI's tests import package syscall for
	// syscall.Mkfifo, and the composition roots' groups refuse these names
	// there.
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

// cliNoEnvOrProcessAllowed are names no cli-no-env-or-process entry may refuse:
// what the CLI is for (it opens the inventory a user names, reads it, creates
// the report and makes room for a temporary one), the process names that are no
// call on a variable, syscall.Mkfifo, which is why the CLI's tests import package
// syscall at all, the names that only extend a refused one (os.Getenvs,
// os.LookupEnvs, os.ExpandEnvs, os.Environments, os.Setenvs, os.StartProcesses,
// os.UserHomeDirs, os.UserConfigDirs, os.UserCacheDirs, os.TempDirs,
// syscall.Getenvs, syscall.Environments, syscall.Execve, syscall.Syscalls,
// syscall.RawSyscalls, syscall.SYSCTL) and catch a pattern that lost its $, and
// a package named os or syscall elsewhere, which is no file of this module's.
var cliNoEnvOrProcessAllowed = []rootName{
	{"os", "os.Open"},
	{"os", "os.OpenFile"},
	{"os", "os.ReadFile"},
	{"os", "os.Create"},
	{"os", "os.CreateTemp"},
	{"os", "os.MkdirTemp"},
	{"os", "os.WriteFile"},
	{"os", "os.MkdirAll"},
	{"os", "os.Stat"},
	{"os", "os.Exit"},
	{"os", "os.Args"},
	{"os", "os.Stdin"},
	{"os", "os.Stdout"},
	{"os", "os.Stderr"},
	{"os", "os.Getpid"},
	{"os", "os.Setenv"},
	{"os", "os.Unsetenv"},
	{"os", "os.Clearenv"},
	{"os", "os.Getenvs"},
	{"os", "os.LookupEnvs"},
	{"os", "os.ExpandEnvs"},
	{"os", "os.Environments"},
	{"os", "os.Setenvs"},
	{"os", "os.StartProcesses"},
	{"os", "os.UserHomeDirs"},
	{"os", "os.UserConfigDirs"},
	{"os", "os.UserCacheDirs"},
	{"os", "os.TempDirs"},
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

// TestCLIImportsProbe runs depguard with the repository's .golangci.yml on a
// probe module laid out like the repository (testdata/cliimports/mod), whose
// internal/cli holds the imports the CLI's rule must refuse and every import it
// must allow. The rule is a strict allow list (issue #152): it names the module's
// packages and exactly the standard packages internal/cli imports today, each
// ending in $, and refuses everything else, so a package a root may not use is
// no import of the CLI's either.
//
// imports.go imports os/exec, syscall and unsafe, each with a want that names no
// rule: before the change $gostd allowed unsafe, and os/exec and syscall were
// refused by deny entries beside the allow list. allowed.go imports the fifteen
// standard packages and the three module packages the CLI imports today with no
// want comment, so an allow entry lost in the rewrite fails here.
//
// make lint runs this probe with SHIFT2EU_LINT_PROBES=require on the host, and
// every entry of CI's lint-target matrix runs the step that sets it.
func TestCLIImportsProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{
		dir:      "cliimports",
		depguard: true,
	})
}

// TestCLINoEnvOrProcessForbidigoPatterns checks the entries of the
// cli-no-env-or-process group, read from .golangci.yml, offline and without
// golangci-lint: every name the CLI may not use is refused by an entry of the
// group, no name it may use is refused by one, and every entry refuses a listed
// name. The probe (TestCompositionRootForbidigoProbe) proves how forbidigo
// renders a name; this pins the patterns and their packages over more names than
// a probe can hold, and it is the pin a reviewer reads when the group changes.
func TestCLINoEnvOrProcessForbidigoPatterns(t *testing.T) {
	t.Parallel()

	entries := groupEntries(t, moduleRoot(t), cliNoEnvOrProcessTag)
	refuses := func(e forbidEntry, n rootName) bool {
		return e.pkg.MatchString(n.pkg) && e.pattern.MatchString(n.name)
	}

	for _, r := range cliNoEnvOrProcessRefused {
		var msgs []string

		for _, e := range entries {
			if refuses(e, r) {
				msgs = append(msgs, e.msg)
			}
		}

		if !slices.ContainsFunc(msgs, func(m string) bool { return strings.HasPrefix(m, cliNoEnvOrProcessTag+": ") }) {
			t.Errorf("%s (pkg %q) must be refused by an entry of group %s; refused by %q", r.name, r.pkg, cliNoEnvOrProcessTag, msgs)
		}
	}

	for _, n := range cliNoEnvOrProcessAllowed {
		for _, e := range entries {
			if refuses(e, n) {
				t.Errorf("%s (pkg %q) must pass the CLI, but the entry with msg %q refuses it", n.name, n.pkg, e.msg)
			}
		}
	}

	for _, e := range entries {
		if !slices.ContainsFunc(cliNoEnvOrProcessRefused, func(r rootName) bool { return refuses(e, r) }) {
			t.Errorf("the entry with msg %q refuses no name of the group's list; add the names it refuses", e.msg)
		}
	}
}
