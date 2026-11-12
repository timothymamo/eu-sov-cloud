package lint_test

import "testing"

// TestAppTestsUseFakesNotAdaptersProbe runs depguard with the repository's
// .golangci.yml on a probe module laid out like the repository
// (testdata/apptestimports/mod), whose application layer's _test.go files hold
// the imports the rule must refuse (probe_test.go) and every import it must
// allow (allow_test.go). The rule is a strict allow list (issue #144): it names
// what the application layer's tests may import and refuses everything else, so
// it is the only depguard rule that holds internal/app's _test.go files
// (app-uses-ports-not-adapters excludes them with !$test).
//
// probe_test.go pins both halves. The packages an entry of the deny list already
// refuses with a reason (net, net/http, net/textproto, crypto/tls, syscall,
// plugin, os, os/exec) must stay refused, and so must the packages that do I/O
// without importing os or net (io/ioutil, log/syslog, text/template,
// html/template, archive/zip, debug/elf, debug/buildinfo, go/build, go/importer,
// runtime/debug), the four the last reviewer and auditor round added (go/parser,
// mime/multipart, runtime/coverage, unsafe) and an adapter. Those wants name only
// the list, so the entry that refuses one may pick its reason. Two more are the
// point of issue #144: encoding/xml and expvar are on no list at all, so the lax
// deny list let them through, and runtime and go/build/constraint are pure and
// still not needed by the application layer's tests, so only a strict allow list
// refuses them. All four carry a want in this probe. fmt and time carry one too,
// and they are the minimal list's own (the review's finding B1 and the
// auditor's L-2): the rule used to share the domain's whole pure list, so fmt
// and time — which print, scan, read zoneinfo files and read the clock — were
// allowed in a test, where the forbidigo group that refuses those calls does not
// hold. The allow list names neither now.
//
// allow_test.go imports everything the allow list names: the standard packages
// the application layer's tests need, go-cmp, the application package under test,
// a domain package and internal/testutil. No line there carries a want, so an
// allow entry that lost its place, or a deny entry that shadows an allowed
// package, fails the probe.
//
// Every issue must match a want and every want must get one, so a rule that
// stops firing fails here. The probe is a _test.go file to prove the rule lints
// test files at all.
//
// The probe runs twice: once for the host's platform with cgo on, as make lint
// loads the module, and once for a release target that is not the host's, with
// cgo off as the release builds it (issue #144's review, finding S2). depguard
// reads a file's imports, so the second run holds that the rule fires for a
// build the host does not make, on the platform a release ships. windows is no
// target for this probe: log/syslog has no Dial there, so the refused import
// would not type-check.
//
// make lint runs this probe with SHIFT2EU_LINT_PROBES=require on the host, and
// every entry of CI's lint-target matrix runs the step that sets it.
func TestAppTestsUseFakesNotAdaptersProbe(t *testing.T) {
	t.Parallel()

	// The allow test imports go-cmp, the one third-party package the rule
	// names; the probe is offline, so the harness writes this stub module and
	// replaces the real path with it.
	stubs := map[string]string{"go-cmp": "github.com/google/go-cmp"}

	runForbidigoProbe(t, forbidigoProbe{
		dir:      "apptestimports",
		depguard: true,
		stubs:    stubs,
	})
	runForbidigoProbe(t, forbidigoProbe{
		dir:      "apptestimports",
		depguard: true,
		stubs:    stubs,
		goos:     "darwin",
		goarch:   "amd64",
	})
}
