package lint_test

import "testing"

// TestE2EImportsProbe runs depguard with the repository's .golangci.yml on a
// probe module laid out like the repository (testdata/e2eimports/mod), whose
// test/e2e holds a production file and a _test.go file (issue #185).
//
// test/e2e/doc.go carries no build tag, so test/e2e is a production package a
// composition root may import, and depguard's strict rule test-docs-are-pure
// holds every file of it to an allow list of nothing. imports.go imports os/exec,
// unsafe and os/user, each with a want that names no rule, so the probe stays
// true whether test-docs-are-pure or another rule refuses them, and an allow list
// that named one of the three, or a files: list that lost "**/test/e2e/**",
// fails here. That is what closes os/exec: a command holds an exec.Cmd with a
// nil Env, which inherits the whole environment of the importing process.
//
// runs_test.go imports os/exec with no want comment: !$test takes the tier's
// _test.go files out of test-docs-are-pure, so they stay on e2e-is-black-box and
// keep building and running the binary, as issue #185 requires.
//
// make lint runs this probe with SHIFT2EU_LINT_PROBES=require on the host, and
// every entry of CI's lint-target matrix runs the step that sets it.
func TestE2EImportsProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{
		dir:      "e2eimports",
		depguard: true,
	})
}
