package lint_test

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/test/internal/shellcheck"
)

// The checker these scans run, and its unit tests, live in
// test/internal/shellcheck (issue #177): test/lint holds doc.go and _test.go
// files only.

// TestTrackedShellScriptsNameTheirShellcheckCodes reads every shell script git
// tracks, IsShellScript deciding which one is a script: the files ending in
// .sh and the files whose first line runs sh or bash, the hooks under .githooks
// among them. Each shellcheck disable directive in one of them must name the
// codes it silences and give its reason (issue #164); a directive that would
// silence every check, or name none, hides a finding no reader can look up.
func TestTrackedShellScriptsNameTheirShellcheckCodes(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)

	var checked, withSuffix []string
	for _, rel := range trackedFiles(t) {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // tracked but not in this checkout, as a sparse one has it
			}
			t.Fatalf("read %s: %v", rel, err)
		}
		if strings.HasSuffix(rel, ".sh") {
			withSuffix = append(withSuffix, rel)
		}
		if !shellcheck.IsShellScript(rel, src) {
			continue
		}
		checked = append(checked, rel)
		for _, problem := range shellcheck.CheckDirectives(rel, src) {
			t.Error(problem)
		}
	}

	// Guard the walk itself: a repository with no script in scope, or a scan
	// that skipped the .sh files, would pass this test whatever the scripts
	// say.
	if len(checked) == 0 {
		t.Fatal("found no tracked shell script to check; IsShellScript or the git walk is broken")
	}
	for _, rel := range withSuffix {
		if !slices.Contains(checked, rel) {
			t.Errorf("the scan skipped %s, a tracked .sh file", rel)
		}
	}
	for _, rel := range []string{".githooks/pre-commit", ".githooks/pre-push"} {
		if !slices.Contains(checked, rel) {
			t.Errorf("the scan skipped %s, whose #!/bin/sh shebang puts it in scope", rel)
		}
	}
	t.Logf("checked %d tracked shell scripts", len(checked))
}

// TestTrackedWorkflowsNameTheirShellcheckCodes extends the scan above to the
// workflows under .github/workflows, .yml and .yaml alike: actionlint hands
// every run: script to shellcheck (issue #164, ADR 0022), so a disable
// directive in one of them hides a finding from the same lint, and a directive
// that names no code or gives no reason hides it from the reader. A workflow is
// not a shell script, so IsShellScript never reaches it; the whole file is
// checked, its YAML included, because the script text inside it may be indented
// by any amount.
func TestTrackedWorkflowsNameTheirShellcheckCodes(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	var scanned []string
	for _, rel := range trackedFiles(t) {
		if !strings.HasPrefix(rel, ".github/workflows/") || (!strings.HasSuffix(rel, ".yml") && !strings.HasSuffix(rel, ".yaml")) {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // tracked but not in this checkout, as a sparse one has it
			}
			t.Fatalf("read %s: %v", rel, err)
		}
		scanned = append(scanned, rel)
		for _, problem := range shellcheck.CheckDirectives(rel, src) {
			t.Error(problem)
		}
	}

	// Guard the walk itself: a workflow scan that found nothing would pass
	// whatever the workflows say.
	if len(scanned) == 0 {
		t.Fatal("found no tracked workflow to check; the .github/workflows walk is broken")
	}
	for _, rel := range []string{".github/workflows/ci.yml"} {
		if !slices.Contains(scanned, rel) {
			t.Errorf("the scan skipped %s, a tracked workflow", rel)
		}
	}
	t.Logf("checked %d tracked workflows", len(scanned))
}

// TestNoTrackedConfigSilencesShellcheckOrActionlint refuses a configuration
// file that would switch a lint off rather than fix what it reports: ShellCheck
// reads .shellcheckrc, or shellcheckrc, from the working directory upwards, and
// actionlint reads .github/actionlint.yml, which can ignore whole rules or
// paths (issue #164, ADR 0022). Neither is tracked today, so this test guards
// the rule: the branch that adds one fails here.
func TestNoTrackedConfigSilencesShellcheckOrActionlint(t *testing.T) {
	t.Parallel()

	refused := []string{".shellcheckrc", "shellcheckrc", ".github/actionlint.yml", ".github/actionlint.yaml"}
	for _, rel := range trackedFiles(t) {
		if slices.Contains(refused, rel) || slices.Contains(refused, path.Base(rel)) {
			t.Errorf("git tracks %s, which silences shellcheck or actionlint; fix the finding it would hide instead", rel)
		}
	}
}

// trackedFiles returns every path git tracks in the module root, slash
// separated, as the repository scans above read the index.
func trackedFiles(t *testing.T) []string {
	t.Helper()

	root := moduleRoot(t)

	// -z keeps a path with a space in it whole, as test/architecture reads the
	// index too.
	out, err := exec.CommandContext(t.Context(), "git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("git ls-files in %s: %v", root, err)
	}

	var files []string
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel != "" {
			files = append(files, rel)
		}
	}
	return files
}
