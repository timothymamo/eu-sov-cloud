package lint_test

import "testing"

// TestVersionImportsProbe runs depguard with the repository's .golangci.yml on a
// probe module laid out like the repository (testdata/versionimports/mod), whose
// internal/version imports the packages the version-is-build-metadata rule must
// refuse and every import it must allow (issue #152, the security audit's M-3).
//
// internal/version is the build metadata both driving adapters print, and both of
// their strict import rules allow it, so it is a package a composition root
// reaches without either rule following what it does: a root that may not read
// the environment or start a process could hand either to it. The rule is a
// strict allow list of exactly what the package imports today, each entry ending
// in $: fmt. imports.go therefore imports os and os/exec, each with a want that
// names no rule, and allowed.go imports fmt with none, so a rule that lost an
// entry, or a $gostd that came back, fails here.
//
// make lint runs this probe with SHIFT2EU_LINT_PROBES=require on the host, and
// every entry of CI's lint-target matrix runs the step that sets it.
func TestVersionImportsProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{
		dir:      "versionimports",
		depguard: true,
	})
}
