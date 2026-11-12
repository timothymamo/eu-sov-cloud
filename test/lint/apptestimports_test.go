package lint_test

import "testing"

// TestAppTestsUseFakesNotAdaptersProbe runs depguard with the repository's
// .golangci.yml on a probe module laid out like the repository
// (testdata/apptestimports/mod), whose application layer's _test.go file
// imports the packages the rule must refuse. The rule is lax: only its deny
// list decides, and it is the only depguard rule that holds internal/app's
// _test.go files (app-uses-ports-not-adapters excludes them with !$test). The
// probe pins both halves: the standard packages an entry already refuses with
// a reason (net, net/http, net/textproto, crypto/tls, syscall, plugin, os,
// os/exec) must stay refused, and the packages that do I/O without importing os
// or net (io/ioutil, log/syslog, text/template, html/template, archive/zip,
// debug/elf, debug/buildinfo, go/build, go/importer, runtime/debug) must be
// refused too, so its want names only the list and the entry may pick its
// reason. The last reviewer and auditor round added four to that half: go/parser
// (ParseFile and ParseDir read files), mime/multipart (ReadForm writes temp
// files), runtime/coverage (WriteMetaDir and WriteCountersDir write files) and
// unsafe (//go:linkname reaches a denied package's functions without an import).
// The pure packages beside them (net/url, net/netip, strings, context, testing,
// runtime) must not be reported, and every issue must match a want, so a rule
// that stops firing fails here. go/build/constraint is pure and with no want on
// purpose: it pins that the go/build entry stays exact (go/build$), since a bare
// go/build prefix entry would refuse it and fail this probe. The probe is a
// _test.go file to prove the rule lints test files at all.
//
// make lint runs this probe with SHIFT2EU_LINT_PROBES=require on the host, and
// every entry of CI's lint-target matrix runs the step that sets it.
func TestAppTestsUseFakesNotAdaptersProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{dir: "apptestimports", depguard: true})
}
