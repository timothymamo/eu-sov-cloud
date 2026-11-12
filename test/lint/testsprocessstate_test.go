package lint_test

import (
	"slices"
	"strings"
	"testing"
)

// testsNoProcessStateTag is the tag of forbidigo's tests-no-process-state group
// (issue #153): t.Setenv and t.Chdir, which no test of this repository and no
// helper of internal/testutil may call. t.Setenv changes the environment of the
// whole test binary — every test in the process then sees the variable, a
// credential's name among them (CLAUDE.md rule 3) — and t.Chdir changes the
// directory the process runs in, which a t.Parallel test shares. A test that
// needs a value passes the case it wants to the code under test, through a fake
// or a parameter, and the CLI and the HTTP API take their inputs from flags and
// the request.
//
// The group names the calls on the four receivers a testing helper hands a test —
// *testing.T, *testing.B, *testing.F and the testing.TB interface — because a
// helper that takes a testing.TB leaves the call one indirection away, and
// forbidigo's analyze-types matching renders the method on the type it is
// declared on, not on the receiver's name. A method value (t.Setenv with no call)
// is refused too: taken alone, it hands the change to whoever the test passes it
// to.
//
// It holds for the _test.go files of internal/app, internal/testutil, internal/cli
// and internal/httpapi and for every file under internal/testutil, which its
// linters.exclusions.rules say and TestForbidigoExclusionsDropOneGroup checks. It
// does not hold everywhere: test/lint and test/architecture set the go command's
// own variables (GOCACHE, GOFLAGS, HOME) on purpose, in a test binary of their
// own, to run the go command in a known environment.
//
// t.TempDir is deliberately allowed, and so are os.MkdirTemp("", ...) and
// os.CreateTemp("", ...) (TestTestsNoProcessStateProbe holds all three): they read
// $TMPDIR implicitly through os.TempDir and return a fresh temporary path, never a
// home, config or credential path, and t.TempDir itself goes through the same
// read, so refusing them would leave no way to get a temporary directory.
// os.TempDir named directly stays refused where testutil-no-env refuses it today.
const testsNoProcessStateTag = "tests-no-process-state"

// testsNoProcessStateRefused lists the rendered names the group must refuse, as
// forbidigo's analyze-types matching renders them: package testing, the type the
// method is declared on, and the method. A name is refused when an entry of the
// group matches its pkg and its rendered name; the wording of a msg is not pinned
// here, the tag is.
var testsNoProcessStateRefused = []rootName{
	{"testing", "testing.T.Setenv"},
	{"testing", "testing.T.Chdir"},
	{"testing", "testing.B.Setenv"},
	{"testing", "testing.B.Chdir"},
	{"testing", "testing.F.Setenv"},
	{"testing", "testing.F.Chdir"},
	{"testing", "testing.TB.Setenv"},
	{"testing", "testing.TB.Chdir"},
}

// testsNoProcessStateAllowed are names no tests-no-process-state entry may
// refuse: the temporary directory a test takes, which reads $TMPDIR the way
// os.MkdirTemp does and is the one way a test gets a directory, the rest of what
// testing hands a test, the names that only extend a refused one
// (testing.T.Setenvs, testing.T.Chdirs, testing.TB.Setenvs, testing.TB.Chdirs)
// and catch a pattern that lost its $, and a package named testing elsewhere,
// which is no test of this module's.
var testsNoProcessStateAllowed = []rootName{
	{"testing", "testing.T.TempDir"},
	{"testing", "testing.B.TempDir"},
	{"testing", "testing.F.TempDir"},
	{"testing", "testing.TB.TempDir"},
	{"testing", "testing.T.Helper"},
	{"testing", "testing.T.Parallel"},
	{"testing", "testing.T.Run"},
	{"testing", "testing.T.Log"},
	{"testing", "testing.T.Logf"},
	{"testing", "testing.T.Cleanup"},
	{"testing", "testing.T.Deadline"},
	{"testing", "testing.T.Context"},
	{"testing", "testing.T.Setenvs"},
	{"testing", "testing.T.Chdirs"},
	{"testing", "testing.TB.Setenvs"},
	{"testing", "testing.TB.Chdirs"},
	{"example.com/othertesting", "testing.T.Setenv"},
}

// TestTestsNoProcessStateProbe runs forbidigo with the repository's .golangci.yml
// on a probe module laid out like the repository (testdata/testsprocessstate/mod),
// whose _test.go files hold the lines the group must refuse on each receiver, and
// the lines it must keep allowing.
//
// internal/app/analysis/probe_test.go holds t.Setenv and t.Chdir on *testing.T,
// *testing.B, *testing.F and through testing.TB, the method value, and the
// testing methods that must pass; internal/testutil holds the same on a *testing.T
// and a testing.TB and temp.go holds os.MkdirTemp("", ...) and
// os.CreateTemp("", ...); the roots' and the driving adapters' _test.go files
// (cmd/shift2eu, cmd/shift2eu-server, internal/cli, internal/httpapi) hold the two
// calls on a *testing.T, because the limits the roots' tests are held to are the
// ones their driving adapters' tests are held to as well. Every line that ends in
// a want comment must get an issue of the group, and no issue may land on a line
// without one, so a group that stops firing, or one that reaches t.TempDir,
// os.MkdirTemp or os.CreateTemp, fails here.
//
// make lint runs this probe with SHIFT2EU_LINT_PROBES=require on the host, and
// every entry of CI's lint-target matrix runs the step that sets it.
func TestTestsNoProcessStateProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{
		dir:  "testsprocessstate",
		tags: []string{testsNoProcessStateTag},
	})
}

// TestTestsNoProcessStateForbidigoPatterns checks the entries of the
// tests-no-process-state group, read from .golangci.yml, offline and without
// golangci-lint: every name a test may not use is refused by an entry of the
// group, no name it may use is refused by one, and every entry refuses a listed
// name. The probe proves how forbidigo renders a name; this pins the patterns and
// their packages over more names than a probe can hold, and it is the pin a
// reviewer reads when the group changes.
func TestTestsNoProcessStateForbidigoPatterns(t *testing.T) {
	t.Parallel()

	entries := groupEntries(t, moduleRoot(t), testsNoProcessStateTag)
	refuses := func(e forbidEntry, n rootName) bool {
		return e.pkg.MatchString(n.pkg) && e.pattern.MatchString(n.name)
	}

	for _, r := range testsNoProcessStateRefused {
		var msgs []string

		for _, e := range entries {
			if refuses(e, r) {
				msgs = append(msgs, e.msg)
			}
		}

		if !slices.ContainsFunc(msgs, func(m string) bool { return strings.HasPrefix(m, testsNoProcessStateTag+": ") }) {
			t.Errorf("%s (pkg %q) must be refused by an entry of group %s; refused by %q", r.name, r.pkg, testsNoProcessStateTag, msgs)
		}
	}

	for _, n := range testsNoProcessStateAllowed {
		for _, e := range entries {
			if refuses(e, n) {
				t.Errorf("%s (pkg %q) must pass a test, but the entry with msg %q refuses it", n.name, n.pkg, e.msg)
			}
		}
	}

	for _, e := range entries {
		if !slices.ContainsFunc(testsNoProcessStateRefused, func(r rootName) bool { return refuses(e, r) }) {
			t.Errorf("the entry with msg %q refuses no name of the group's list; add the names it refuses", e.msg)
		}
	}
}
