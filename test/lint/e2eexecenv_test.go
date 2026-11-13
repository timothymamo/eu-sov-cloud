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

// e2eToolchainCommands lists the command names a test/e2e _test.go file may
// start with the whole environment of the test process. It is the go
// toolchain's alone: the `go build` calls of assess_test.go's buildAndRun and
// the `go list -deps` calls of assess_test.go and server_test.go inherit that
// environment because the go command needs it (GOPATH, the module cache,
// GOFLAGS, a proxy, a toolchain) and print none of it (issue #195). Every other
// command must set its Env: exec.Cmd reads a nil Env as "inherit the whole
// environment", where the agents' keys and tokens live (CLAUDE.md rule 3), and
// what the child prints can reach a t.Log call or a golden file. The exemption
// is keyed on the command name the source writes, so a `go` call is let through
// whole; the e2e-no-env forbidigo group and review hold the rest.
var e2eToolchainCommands = []string{"go"}

// e2eExecExempt reports whether the e2e tier may start the command named name
// with no Env of its own (e2eToolchainCommands).
func e2eExecExempt(name string) bool { return slices.Contains(e2eToolchainCommands, name) }

// TestE2ETestsSetExecEnv holds every test/e2e _test.go file to the rule that a
// command it runs carries an Env of its own. The tier imports os/exec on
// purpose, so forbidigo's e2e-no-env group cannot refuse the import; it refuses
// (*exec.Cmd).Environ, the one call that dumps the environment, but nothing
// refuses a command built with a nil Env, which inherits all of it. Every run of
// the binary sets cmd.Env today, and only review keeps it so (issue #195).
// execenv.CheckExecEnv is the checker; the go toolchain's calls are exempt
// (e2eToolchainCommands).
func TestE2ETestsSetExecEnv(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	dir := filepath.Join(root, "test", "e2e")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	var scanned []string
	for _, entry := range entries {
		// Every _test.go file of the tier: doc.go is the package's only
		// production file, and a subdirectory holds fixtures.
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		rel := "test/e2e/" + entry.Name()
		src, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		scanned = append(scanned, rel)
		findings, err := execenv.CheckExecEnv(rel, src, e2eExecExempt)
		if err != nil {
			t.Fatalf("check %s: %v", rel, err)
		}
		for _, f := range findings {
			t.Error(findingLine(f))
		}
	}

	// Guard the scan itself: a walk that found no test file, or one that lost
	// the files the toolchain calls live in, would pass whatever they say.
	for _, rel := range []string{
		"test/e2e/assess_test.go",
		"test/e2e/inventory_guide_test.go",
		"test/e2e/server_test.go",
	} {
		if !slices.Contains(scanned, rel) {
			t.Errorf("the scan skipped %s, a _test.go file of the e2e tier", rel)
		}
	}
	t.Logf("checked %d e2e test files", len(scanned))
}

// TestTestTreeNamesNoProcEnviron holds every Go file under test/ to the rule
// that no string literal in it names a /proc/<something>/environ path: reading
// that file hands over the whole environment of the test process without
// starting a child, and the e2e-no-env forbidigo group allows os.ReadFile, so
// nothing else refuses it (issue #195).
//
// The walk skips nothing, testdata directories included: a literal there would
// be a probe of some other rule, and the probes of this one are .go.txt files
// under test/internal/execenv/testdata, which are no Go files and which this
// scan therefore never meets.
func TestTestTreeNamesNoProcEnviron(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)

	var scanned []string
	refused, err := walkGoFiles(filepath.Join(root, "test"), nil, func(rel string, src []byte) error {
		scanned = append(scanned, rel)
		findings, err := execenv.CheckEnvironLiterals("test/"+rel, src)
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

	// Guard the walk itself: it must reach the tier this rule is about and a
	// testdata directory, which it does not skip.
	for _, rel := range []string{"e2e/assess_test.go", "lint/doc.go", "lint/testdata/e2enoenv/mod/test/e2e/probe.go"} {
		if !slices.Contains(scanned, rel) {
			t.Errorf("the scan skipped %s; it reads every Go file under test/", rel)
		}
	}
	t.Logf("checked %d Go files under test/", len(scanned))
}

// findingLine renders one refusal of a checker as the line a reader sees in a
// test failure.
func findingLine(f execenv.Finding) string {
	return fmt.Sprintf("%s:%d: %s", f.File, f.Line, f.Msg)
}
