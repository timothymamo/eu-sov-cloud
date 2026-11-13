package lint_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/test/internal/execenv"
)

// The checker these scans run, and its probes, live in test/internal/execenv
// (issue #195), as the shellcheck rule's do in test/internal/shellcheck:
// test/lint holds doc.go and _test.go files only.

// e2eToolchainCommands lists the commands a test/e2e _test.go file may start
// with the whole environment of the test process, as the name and the
// subcommand the source writes, joined by a space. It is the go toolchain's
// alone: the `go build` calls of assess_test.go's buildAndRun and the
// `go list -deps` calls of assess_test.go and server_test.go inherit that
// environment because the go command needs it (GOPATH, the module cache,
// GOFLAGS, a proxy, a toolchain) and print none of it (issue #195). Every other
// command must set its Env: exec.Cmd reads a nil Env as "inherit the
// environment", where the agents' keys and tokens live (CLAUDE.md rule 3), and
// what the child prints can reach a t.Log call or a golden file. The exemption
// is keyed on the name and the subcommand, both string literals, so a `go run`
// or a `go env` call is let through by nothing; the e2e-no-env forbidigo group
// and review hold the rest.
var e2eToolchainCommands = []string{"go build", "go list"}

// e2eExecExempt reports whether the e2e tier may start the command named name
// with its first argument sub and no Env of its own (e2eToolchainCommands).
func e2eExecExempt(name, sub string) bool {
	return slices.Contains(e2eToolchainCommands, name+" "+sub)
}

// TestE2ETestsSetExecEnv holds every test/e2e _test.go file to the rule that a
// command it runs carries an Env of its own. The tier imports os/exec on
// purpose, so forbidigo's e2e-no-env group cannot refuse the import; it refuses
// (*exec.Cmd).Environ, the one call that dumps the environment, but nothing
// refuses a command built with a nil Env, which inherits all of it. Every run of
// the binary sets cmd.Env today, and only review keeps it so (issue #195).
// execenv.CheckExecEnv is the checker; the go toolchain's calls are exempt
// (e2eToolchainCommands).
//
// The walk is recursive, so a sub-package the tier grows later is scanned too,
// and skips the testdata directories, which hold fixtures and no package of the
// tier.
func TestE2ETestsSetExecEnv(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	dir := filepath.Join(root, "test", "e2e")

	var scanned []string
	refused, err := walkGoFiles(dir, skipTestdataDir, func(rel string, src []byte) error {
		// Every _test.go file of the tier: doc.go is the package's only
		// production file, and TestE2ETestsSetExecEnv holds the tests.
		if !strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		at := "test/e2e/" + rel
		scanned = append(scanned, at)
		findings, err := execenv.CheckExecEnv(at, src, e2eExecExempt)
		if err != nil {
			return fmt.Errorf("check %s: %w", at, err)
		}
		for _, f := range findings {
			t.Error(findingLine(f))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	for _, problem := range refused {
		t.Error(problem)
	}

	// Guard the scan itself: a walk that found no test file, or one that lost
	// the files the toolchain calls live in, would pass whatever they say, and
	// one that descended into a testdata directory would read fixtures as if
	// they were the tier's.
	for _, rel := range []string{
		"test/e2e/assess_test.go",
		"test/e2e/inventory_guide_test.go",
		"test/e2e/server_test.go",
	} {
		if !slices.Contains(scanned, rel) {
			t.Errorf("the scan skipped %s, a _test.go file of the e2e tier", rel)
		}
	}
	for _, at := range scanned {
		if strings.Contains(at, "/testdata/") {
			t.Errorf("the scan read %s, a file under testdata, which holds fixtures and no package of the e2e tier", at)
		}
	}
	t.Logf("checked %d e2e test files", len(scanned))
}

// skipTestdataDir is walkGoFiles' skip function for the e2e tier: a testdata
// directory holds fixtures, which are no Go file of the tier the rule is about.
func skipTestdataDir(_ *os.Root, _, name string) (bool, error) {
	return name == "testdata", nil
}

// TestTestTreeNamesNoProcEnviron holds every Go file under test/ to the rule
// that no string literal in it names the environment of a process: reading that
// file hands over the whole environment of the test process without starting a
// child, and the e2e-no-env forbidigo group allows os.ReadFile, so nothing else
// refuses it (issue #195).
//
// The tier decides the rule (execenv.Tier): under test/e2e, which holds no
// fixture literals, any literal that names a path into /proc or /dev/fd is
// refused; elsewhere in test/, where a probe's source and expected message are
// literals of their own, only a literal whose whole value names a
// /proc/<something>/environ file is.
//
// The walk skips nothing, testdata directories included: a literal there would
// be a probe of some other rule, and the probes of this one are .go.txt files
// under test/internal/execenv/testdata, which are no Go files and which this
// scan therefore never meets. One Go file is left out by name
// (environPathsPinnedBy), and the scan fails if the name matches no file, so a
// rename cannot widen the exception quietly.
func TestTestTreeNamesNoProcEnviron(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)

	var scanned []string
	pinned := 0
	refused, err := walkGoFiles(filepath.Join(root, "test"), nil, func(rel string, src []byte) error {
		if rel == environPathsPinnedBy {
			pinned++
			return nil
		}
		scanned = append(scanned, rel)
		tier := execenv.TierTest
		if strings.HasPrefix(rel, "e2e/") {
			tier = execenv.TierE2E
		}
		findings, err := execenv.CheckEnvironLiterals("test/"+rel, src, tier)
		if err != nil {
			return fmt.Errorf("check %s: %w", rel, err)
		}
		for _, f := range findings {
			t.Error(findingLine(f))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk test/: %v", err)
	}
	// walkGoFiles refuses a Go file that is not a regular file (whyNotRegular);
	// an unread file would hide the literals inside it.
	for _, problem := range refused {
		t.Error(problem)
	}

	// Guard the exception: exactly one file is left out, and it is the file that
	// pins the matcher this rule runs on.
	if pinned != 1 {
		t.Errorf("the walk met %s %d times, want it once; the exception of that file is stale", environPathsPinnedBy, pinned)
	}

	// Guard the walk itself: it must reach the tier this rule is about and a
	// testdata directory, which it does not skip.
	for _, rel := range []string{"e2e/assess_test.go", "lint/doc.go", "lint/testdata/e2enoenv/mod/test/e2e/probe.go"} {
		if !slices.Contains(scanned, rel) {
			t.Errorf("the scan skipped %s; it reads every Go file under test/", rel)
		}
	}
	t.Logf("checked %d Go files under test/", len(scanned))
}

// environPathsPinnedBy is the one Go file under test/ that
// TestTestTreeNamesNoProcEnviron leaves out, as a path relative to test/: the
// file that pins execenv.EnvironmentPathIn, the matcher of the e2e tier's rule,
// which names every path the rule refuses as a value, a bare literal being
// exactly what the rule refuses. Its sibling execenv_test.go holds no path of
// its own: the probes of both files are .go.txt files under testdata, which are
// no Go files and which the walk therefore never reads.
const environPathsPinnedBy = "internal/execenv/environmentpath_test.go"

// findingLine renders one refusal of a checker as the line a reader sees in a
// test failure.
func findingLine(f execenv.Finding) string {
	return fmt.Sprintf("%s:%d: %s", f.File, f.Line, f.Msg)
}
