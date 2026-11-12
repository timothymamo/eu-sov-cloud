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
// os.ExpandEnv), to dump the whole environment (os.Environ), to write one
// (os.Setenv, os.Unsetenv, os.Clearenv) and to start a process
// (os.StartProcess), and the four lookups that locate a directory from an
// unnamed variable (os.UserHomeDir, os.UserConfigDir, os.UserCacheDir,
// os.TempDir): a CLI that builds the path to ~/.config/dot-agent-deck/daemon.env
// or ~/.aws/credentials names no variable while it reads one. It holds for files
// under internal/cli/, _test.go files included, which its
// linters.exclusions.rules say and TestForbidigoExclusionsDropOneGroup checks.
//
// It names no package: syscall, unsafe and os/exec are refused at the import by
// the CLI's strict allow list (cli-gets-adapters-from-cmd, issue #152), which
// TestCLIImportsProbe lints. The calls the CLI is for — os.Open, os.ReadFile,
// os.Create and os.CreateTemp, the inventory a user names and the report the
// user asked for — are no calls of this group.
const cliNoEnvOrProcessTag = "cli-no-env-or-process"

// cliNoEnvOrProcessRefused lists every name the group must refuse. A name is
// refused when an entry of the group matches its pkg and its rendered name; the
// wording of a msg is not pinned here, the tag is.
var cliNoEnvOrProcessRefused = []rootName{
	{"os", "os.Getenv"},
	{"os", "os.LookupEnv"},
	{"os", "os.ExpandEnv"},
	{"os", "os.Environ"},
	{"os", "os.Setenv"},
	{"os", "os.Unsetenv"},
	{"os", "os.Clearenv"},
	{"os", "os.StartProcess"},
	{"os", "os.UserHomeDir"},
	{"os", "os.UserConfigDir"},
	{"os", "os.UserCacheDir"},
	{"os", "os.TempDir"},
}

// cliNoEnvOrProcessAllowed are names no cli-no-env-or-process entry may refuse:
// what the CLI is for (it opens the inventory a user names, reads it, creates
// the report and makes room for a temporary one), the process names that are no
// call on a variable, the names that only extend a refused one (os.Getenvs,
// os.LookupEnvs, os.ExpandEnvs, os.Environments, os.Setenvs, os.StartProcesses,
// os.UserHomeDirs, os.UserConfigDirs, os.UserCacheDirs, os.TempDirs) and catch a
// pattern that lost its $, and a package named os elsewhere, which is no file of
// this module's.
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
	{"example.com/otheros", "os.Getenv"},
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
