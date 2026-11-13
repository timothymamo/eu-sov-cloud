package lint_test

import (
	"slices"
	"strings"
	"testing"
)

// e2eNoEnvTag is the tag of forbidigo's e2e-no-env group (issue #167): the calls
// the e2e tier may not make. depguard's e2e-is-black-box allows $gostd, so os and
// syscall, which read the environment, hand over all of it, start a process with
// it, and locate a home, config, cache or temp directory from an unnamed variable.
// test/e2e's doc.go carries no build tag, so a composition root can import the
// package, and test/architecture's TestEveryOsAllowingRuleHasAnEnvironmentGroup
// reads $gostd as allowing os and syscall both: the group refuses the reads of
// both, in the e2e tests too. test-docs-are-pure holds doc.go to errors$ as well.
//
// It holds for files under test/e2e/ only, which its linters.exclusions.rules rule
// says and TestForbidigoExclusionsDropOneGroup checks.
const e2eNoEnvTag = "e2e-no-env"

// e2eNoEnvRefused lists every name the group must refuse: the os reads
// testutil-no-env, cli-no-env-or-process and inventoryfile-no-env refuse, and the
// syscall reads, the ones linux and the ones windows reach the environment or
// start a process with. A name is refused when an entry of the group matches its
// pkg and its rendered name; the wording of a msg is not pinned here, the tag is.
var e2eNoEnvRefused = []rootName{
	{"os", "os.Getenv"},
	{"os", "os.LookupEnv"},
	{"os", "os.ExpandEnv"},
	{"os", "os.Environ"},
	{"os", "os.StartProcess"},
	{"os", "os.UserHomeDir"},
	{"os", "os.UserConfigDir"},
	{"os", "os.UserCacheDir"},
	{"os", "os.TempDir"},
	{"syscall", "syscall.AllThreadsSyscall"},
	{"syscall", "syscall.AllThreadsSyscall6"},
	{"syscall", "syscall.CreateProcess"},
	{"syscall", "syscall.CreateProcessAsUser"},
	{"syscall", "syscall.Exec"},
	{"syscall", "syscall.Environ"},
	{"syscall", "syscall.ForkExec"},
	{"syscall", "syscall.GetEnvironmentStrings"},
	{"syscall", "syscall.GetEnvironmentVariable"},
	{"syscall", "syscall.Getenv"},
	{"syscall", "syscall.GetProcAddress"},
	{"syscall", "syscall.LoadDLL"},
	{"syscall", "syscall.LoadLibrary"},
	{"syscall", "syscall.MustLoadDLL"},
	{"syscall", "syscall.NewLazyDLL"},
	{"syscall", "syscall.RawSyscall"},
	{"syscall", "syscall.RawSyscall6"},
	{"syscall", "syscall.StartProcess"},
	{"syscall", "syscall.Syscall"},
	{"syscall", "syscall.Syscall12"},
	{"syscall", "syscall.Syscall15"},
	{"syscall", "syscall.Syscall18"},
	{"syscall", "syscall.Syscall6"},
	{"syscall", "syscall.Syscall9"},
	{"syscall", "syscall.SyscallN"},
}

// e2eNoEnvAllowed are names no e2e-no-env entry may refuse: what the e2e tests
// build and run the binary with (os/exec, a signal to stop the server, a golden
// file read), the writes, whose group is tests-no-process-state, the names that
// only extend a refused one and catch a pattern that lost its $, and a package
// named os or syscall elsewhere.
var e2eNoEnvAllowed = []rootName{
	{"os/exec", "exec.Command"},
	{"os/exec", "exec.CommandContext"},
	{"os", "os.ReadFile"},
	{"os", "os.WriteFile"},
	{"os", "os.Open"},
	{"os", "os.Stat"},
	{"os", "os.MkdirTemp"},
	{"os", "os.Setenv"},
	{"syscall", "syscall.SIGTERM"},
	{"syscall", "syscall.SIGINT"},
	{"syscall", "syscall.Kill"},
	{"syscall", "syscall.Setenv"},
	{"os", "os.Getenvs"},
	{"os", "os.TempDirs"},
	{"syscall", "syscall.Getenvs"},
	{"syscall", "syscall.Execs"},
	{"example.com/otheros", "os.Getenv"},
	{"example.com/othersyscall", "syscall.Getenv"},
}

// TestE2ENoEnvProbe runs forbidigo with the repository's .golangci.yml on a probe
// module laid out like the repository (testdata/e2enoenv/mod), whose test/e2e
// holds the calls the group must refuse and the ones it must keep allowing. Every
// refused line ends in a want comment naming the group, and every entry of the
// group must refuse a line, so a group that stops firing, or one that reaches
// os/exec, fails here.
//
// make lint runs this probe with SHIFT2EU_LINT_PROBES=require on the host, and
// every entry of CI's lint-target matrix runs the step that sets it.
func TestE2ENoEnvProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{
		dir:  "e2enoenv",
		tags: []string{e2eNoEnvTag},
	})
}

// TestE2ENoEnvForbidigoPatterns checks the entries of the e2e-no-env group, read
// from .golangci.yml, offline and without golangci-lint: every name the e2e tier
// may not use is refused by an entry of the group, no name it may use is refused
// by one, and every entry refuses a listed name. The probe proves how forbidigo
// renders a name; this pins the patterns and their packages over more names than
// a probe can hold, the ones a unix probe cannot call (syscall.SyscallN) among
// them.
func TestE2ENoEnvForbidigoPatterns(t *testing.T) {
	t.Parallel()

	entries := groupEntries(t, moduleRoot(t), e2eNoEnvTag)
	refuses := func(e forbidEntry, n rootName) bool {
		return e.pkg.MatchString(n.pkg) && e.pattern.MatchString(n.name)
	}

	for _, r := range e2eNoEnvRefused {
		var msgs []string

		for _, e := range entries {
			if refuses(e, r) {
				msgs = append(msgs, e.msg)
			}
		}

		if !slices.ContainsFunc(msgs, func(m string) bool { return strings.HasPrefix(m, e2eNoEnvTag+": ") }) {
			t.Errorf("%s (pkg %q) must be refused by an entry of group %s; refused by %q", r.name, r.pkg, e2eNoEnvTag, msgs)
		}
	}

	for _, n := range e2eNoEnvAllowed {
		for _, e := range entries {
			if refuses(e, n) {
				t.Errorf("%s (pkg %q) must pass the e2e tier, but the entry with msg %q refuses it", n.name, n.pkg, e.msg)
			}
		}
	}

	for _, e := range entries {
		if !slices.ContainsFunc(e2eNoEnvRefused, func(r rootName) bool { return refuses(e, r) }) {
			t.Errorf("the entry with msg %q refuses no name of the group's list; add the names it refuses", e.msg)
		}
	}
}
