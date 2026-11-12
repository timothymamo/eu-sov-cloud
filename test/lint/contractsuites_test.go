package lint_test

import "testing"

// TestContractSuitesArePureProbe runs depguard with the repository's
// .golangci.yml on a probe module laid out like the repository
// (testdata/contractsuites/mod), whose internal/adapter/source/sourcetest and
// internal/adapter/target/targettest hold the imports the rule must refuse and
// every import it must allow (issue #152, the auditor's finding L-A).
//
// The contract suites are production packages, not _test packages, and any
// _test.go file of the module may import them. Without a rule of its own a suite
// could start a program, read the environment, open a connection or take a
// pointer apart, and every adapter test that imports it would get that with lint
// green. The rule contract-suites-are-pure is therefore a strict allow list of
// what the two suites import today, each entry ending in $.
//
// refused.go in each suite imports net, os, os/exec, syscall and unsafe, each with
// a want naming no rule, so extending the rule or adding a sibling rule with the
// same entries keeps the probe true as long as the import is refused. allowed.go
// imports every package the allow list names, with no want comment, so an entry
// lost in the rewrite fails here.
//
// make lint runs this probe with SHIFT2EU_LINT_PROBES=require on the host, and
// every entry of CI's lint-target matrix runs the step that sets it.
func TestContractSuitesArePureProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{
		dir:      "contractsuites",
		depguard: true,
	})
}
