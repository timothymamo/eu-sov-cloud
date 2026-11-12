package lint_test

import (
	"slices"
	"strings"
	"testing"
)

// testutilNoEnvTag is the tag of forbidigo's testutil-no-env group (issue #144):
// the calls a helper of internal/testutil may not make, which reach the
// environment or hand a child a process. CLAUDE.md rule 3 keeps credentials out
// of the helpers' hands, and the depguard rule testutil-imports allows os, which
// the golden files are read and written with, so no import rule can say it.
//
// The group refuses os.Getenv, os.LookupEnv, os.ExpandEnv, os.Environ and
// os.StartProcess: every read of a named variable, the whole environment, and the
// process start that takes an environment. The writes of a variable —
// os.Setenv, os.Unsetenv and os.Clearenv — are the group
// tests-no-process-state's since issue #153, which refuses them in every file of
// the module but the two harnesses under test/. It refuses os.UserHomeDir, os.UserConfigDir,
// os.UserCacheDir and os.TempDir as well (issue #144's review, finding S1): each
// locates a directory from an unnamed variable, so a helper that builds the path
// to ~/.config/dot-agent-deck/daemon.env or ~/.aws/credentials and reads it never
// names one. It holds for files under internal/testutil/ only, which its
// linters.exclusions.rules say and TestForbidigoExclusionsDropOneGroup checks.
const testutilNoEnvTag = "testutil-no-env"

// testutilNoEnvRefused lists every name the group must refuse: these calls, and
// os.ExpandEnv, which reads the named variables a string interleaves. A name is
// refused when an entry of the group matches its pkg and its rendered name; the
// wording of a msg is not pinned here, the tag is.
var testutilNoEnvRefused = []rootName{
	{"os", "os.Getenv"},
	{"os", "os.LookupEnv"},
	{"os", "os.ExpandEnv"},
	{"os", "os.Environ"},
	{"os", "os.StartProcess"},
	{"os", "os.UserHomeDir"},
	{"os", "os.UserConfigDir"},
	{"os", "os.UserCacheDir"},
	{"os", "os.TempDir"},
}

// testutilNoEnvAllowed are names no testutil-no-env entry may refuse: what the
// helpers really do, which is why os is allowed at all (the golden files are
// read, written and made room for), the process names that are no call on a
// variable, os.Setenv, os.Unsetenv and os.Clearenv, whose group is
// tests-no-process-state since issue #153 (one group owns the writes, so no line
// is reported twice), the names that only extend a refused one (os.Getenvs,
// os.LookupEnvs, os.ExpandEnvs, os.Setenvs, os.StartProcesses, os.UserHomeDirs,
// os.UserConfigDirs, os.UserCacheDirs, os.TempDirs) and catch a pattern that lost
// its $, and a package named os elsewhere, which is no helper of this module's.
var testutilNoEnvAllowed = []rootName{
	{"os", "os.ReadFile"},
	{"os", "os.WriteFile"},
	{"os", "os.MkdirAll"},
	{"os", "os.Exit"},
	{"os", "os.Getpid"},
	{"os", "os.Getpagesize"},
	{"os", "os.Setenv"},
	{"os", "os.Unsetenv"},
	{"os", "os.Clearenv"},
	{"os", "os.Getenvs"},
	{"os", "os.LookupEnvs"},
	{"os", "os.ExpandEnvs"},
	{"os", "os.Setenvs"},
	{"os", "os.StartProcesses"},
	{"os", "os.UserHomeDirs"},
	{"os", "os.UserConfigDirs"},
	{"os", "os.UserCacheDirs"},
	{"os", "os.TempDirs"},
	{"example.com/otheros", "os.Getenv"},
}

// TestTestutilImportsProbe runs depguard and forbidigo with the repository's
// .golangci.yml on a probe module laid out like the repository
// (testdata/testutilimports/mod), whose internal/testutil holds the helpers'
// stand-in and the lines the two rules must refuse.
//
// depguard's rule testutil-imports holds the directory whole, production and
// _test.go files alike, to the imports the helpers make today (issue #144, the
// auditor's finding M-1 and the reviewer's S1). Nothing bounded a helper's
// imports before: an application layer's test may import internal/testutil, so a
// helper using net/http, os/exec or os.Getenv would have given every test of the
// layer I/O with lint green. io.go imports net/http and os/exec, io_test.go
// imports net/http, and every import carries a want naming the rule. golden.go
// imports os and path/filepath with no want comment: those are the imports the
// helpers make, and the rule must keep allowing them.
//
// forbidigo's group testutil-no-env refuses the calls an import rule cannot see
// (env.go): the environment reads and writes, os.StartProcess and the four
// lookups that locate a directory from an unnamed variable, each on a line whose
// want names the group's tag. Every entry of the group must refuse a line, and
// no issue may land on a line without a want.
//
// make lint runs this probe with SHIFT2EU_LINT_PROBES=require on the host, and
// every entry of CI's lint-target matrix runs the step that sets it.
func TestTestutilImportsProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{
		dir:      "testutilimports",
		tags:     []string{testutilNoEnvTag},
		depguard: true,
	})
}

// TestTestutilNoEnvForbidigoPatterns checks the entries of the testutil-no-env
// group, read from .golangci.yml, offline and without golangci-lint: every name
// a helper may not use is refused by an entry of the group, no name it may use
// is refused by one, and every entry refuses a listed name. The probe proves how
// forbidigo renders a name; this pins the patterns and their packages over more
// names than a probe can hold, and it is the pin a reviewer reads when the group
// changes.
func TestTestutilNoEnvForbidigoPatterns(t *testing.T) {
	t.Parallel()

	entries := groupEntries(t, moduleRoot(t), testutilNoEnvTag)
	refuses := func(e forbidEntry, n rootName) bool {
		return e.pkg.MatchString(n.pkg) && e.pattern.MatchString(n.name)
	}

	for _, r := range testutilNoEnvRefused {
		var msgs []string

		for _, e := range entries {
			if refuses(e, r) {
				msgs = append(msgs, e.msg)
			}
		}

		if !slices.ContainsFunc(msgs, func(m string) bool { return strings.HasPrefix(m, testutilNoEnvTag+": ") }) {
			t.Errorf("%s (pkg %q) must be refused by an entry of group %s; refused by %q", r.name, r.pkg, testutilNoEnvTag, msgs)
		}
	}

	for _, n := range testutilNoEnvAllowed {
		for _, e := range entries {
			if refuses(e, n) {
				t.Errorf("%s (pkg %q) must pass a test helper, but the entry with msg %q refuses it", n.name, n.pkg, e.msg)
			}
		}
	}

	for _, e := range entries {
		if !slices.ContainsFunc(testutilNoEnvRefused, func(r rootName) bool { return refuses(e, r) }) {
			t.Errorf("the entry with msg %q refuses no name of the group's list; add the names it refuses", e.msg)
		}
	}
}
