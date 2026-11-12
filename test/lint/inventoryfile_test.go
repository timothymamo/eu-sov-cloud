package lint_test

import (
	"slices"
	"strings"
	"testing"
)

// inventoryFileNoEnvTag is the tag of forbidigo's inventoryfile-no-env group
// (issue #152, the auditor's finding M-B): the calls the inventory-file reader
// may not make. depguard's inventory-file-reader-reads-one-file allows os, which
// the reader opens the file a user names with, and os also reads the environment,
// hands over all of it, starts a process with one, and locates a home, config,
// cache or temp directory from an unnamed variable. A composition root that may
// not read the environment itself may hand the read to the reader it constructs,
// which is why the group holds the package's _test.go files too.
//
// It refuses the same nine names as testutil-no-env and cli-no-env-or-process.
// The writes of a variable (os.Setenv, os.Unsetenv, os.Clearenv) and os.Chdir are
// the group tests-no-process-state's since issue #153, so no line is reported by
// two groups. It holds for files under
// internal/adapter/source/inventoryfile/ only, which its linters.exclusions.rules
// rule says and TestForbidigoExclusionsDropOneGroup checks.
const inventoryFileNoEnvTag = "inventoryfile-no-env"

// inventoryFileNoEnvRefused lists every name the group must refuse. A name is
// refused when an entry of the group matches its pkg and its rendered name; the
// wording of a msg is not pinned here, the tag is.
var inventoryFileNoEnvRefused = []rootName{
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

// inventoryFileNoEnvAllowed are names no inventoryfile-no-env entry may refuse:
// what the reader is for (it opens, reads and stats the file a user names), what
// its own tests make a temporary file and directory with, the writes, whose group
// is tests-no-process-state since issue #153, the names that only extend a
// refused one (os.Getenvs, os.LookupEnvs, os.ExpandEnvs, os.StartProcesses,
// os.UserHomeDirs, os.UserConfigDirs, os.UserCacheDirs, os.TempDirs) and catch a
// pattern that lost its $, and a package named os elsewhere.
var inventoryFileNoEnvAllowed = []rootName{
	{"os", "os.Open"},
	{"os", "os.OpenFile"},
	{"os", "os.ReadFile"},
	{"os", "os.WriteFile"},
	{"os", "os.Stat"},
	{"os", "os.MkdirAll"},
	{"os", "os.ReadDir"},
	{"os", "os.MkdirTemp"},
	{"os", "os.CreateTemp"},
	{"os", "os.Exit"},
	{"os", "os.Getpid"},
	{"os", "os.Getpagesize"},
	{"os", "os.Setenv"},
	{"os", "os.Unsetenv"},
	{"os", "os.Clearenv"},
	{"os", "os.Getenvs"},
	{"os", "os.LookupEnvs"},
	{"os", "os.ExpandEnvs"},
	{"os", "os.StartProcesses"},
	{"os", "os.UserHomeDirs"},
	{"os", "os.UserConfigDirs"},
	{"os", "os.UserCacheDirs"},
	{"os", "os.TempDirs"},
	{"example.com/otheros", "os.Getenv"},
}

// TestInventoryFileNoEnvProbe runs forbidigo with the repository's .golangci.yml
// on a probe module laid out like the repository (testdata/inventoryfile/mod),
// whose internal/adapter/source/inventoryfile holds the calls the group must
// refuse and the ones it must keep allowing. Every refused line ends in a want
// comment naming the group, os.Open and the calls the reader's tests make carry
// no want comment, and every entry of the group must refuse a line, so a group
// that stops firing, or one that reaches os.Open, fails here.
//
// make lint runs this probe with SHIFT2EU_LINT_PROBES=require on the host, and
// every entry of CI's lint-target matrix runs the step that sets it.
func TestInventoryFileNoEnvProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{
		dir:  "inventoryfile",
		tags: []string{inventoryFileNoEnvTag},
	})
}

// TestInventoryFileNoEnvForbidigoPatterns checks the entries of the
// inventoryfile-no-env group, read from .golangci.yml, offline and without
// golangci-lint: every name the reader may not use is refused by an entry of the
// group, no name it may use is refused by one, and every entry refuses a listed
// name. The probe proves how forbidigo renders a name; this pins the patterns and
// their packages over more names than a probe can hold.
func TestInventoryFileNoEnvForbidigoPatterns(t *testing.T) {
	t.Parallel()

	entries := groupEntries(t, moduleRoot(t), inventoryFileNoEnvTag)
	refuses := func(e forbidEntry, n rootName) bool {
		return e.pkg.MatchString(n.pkg) && e.pattern.MatchString(n.name)
	}

	for _, r := range inventoryFileNoEnvRefused {
		var msgs []string

		for _, e := range entries {
			if refuses(e, r) {
				msgs = append(msgs, e.msg)
			}
		}

		if !slices.ContainsFunc(msgs, func(m string) bool { return strings.HasPrefix(m, inventoryFileNoEnvTag+": ") }) {
			t.Errorf("%s (pkg %q) must be refused by an entry of group %s; refused by %q", r.name, r.pkg, inventoryFileNoEnvTag, msgs)
		}
	}

	for _, n := range inventoryFileNoEnvAllowed {
		for _, e := range entries {
			if refuses(e, n) {
				t.Errorf("%s (pkg %q) must pass the inventory-file reader, but the entry with msg %q refuses it", n.name, n.pkg, e.msg)
			}
		}
	}

	for _, e := range entries {
		if !slices.ContainsFunc(inventoryFileNoEnvRefused, func(r rootName) bool { return refuses(e, r) }) {
			t.Errorf("the entry with msg %q refuses no name of the group's list; add the names it refuses", e.msg)
		}
	}
}
