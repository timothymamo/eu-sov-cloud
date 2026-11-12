package lint_test

import "testing"

// TestAdaptersBuiltInCmdProbe runs depguard with the repository's
// .golangci.yml on a probe module laid out like the repository
// (testdata/adaptersbuiltincmd/mod). adapters-are-built-in-cmd must refuse an
// import of an adapter from the HTTP API, one under each adapter subtree
// (report, request, source, target), and an import of a helper under
// adapters/internal/ that the rule does not name, with the helper's own
// message. The two helpers the rule names stay allowed, and the two
// composition roots may import every adapter. An import of each adapter
// subtree from internal/foo, a package no strict rule holds, is refused by
// adapters-are-built-in-cmd alone: the HTTP API's imports are refused by
// httpapi-gets-adapters-from-cmd too, so without that package the probe would
// stay green if the lax rule stopped firing. Every want on the probe is on a
// line the rule must refuse, and every issue must match a want, so a rule that
// stops firing fails here; that is how the shadowing in issue #119 went
// unnoticed. make lint runs this probe with SHIFT2EU_LINT_PROBES=require on
// the host, and every entry of CI's lint-target matrix runs the step that sets
// it.
func TestAdaptersBuiltInCmdProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{dir: "adaptersbuiltincmd", depguard: true})
}
