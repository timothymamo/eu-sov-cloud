// Package architecture holds the architecture checks that depguard cannot make
// (issue #40, ADR 0012). depguard reads the imports of the Go files
// golangci-lint analyses and picks the rule for a file by its file name, so a
// file name that does not say where the code is gets past it: an assembly file
// that a bodyless Go declaration calls, a line directive that names another
// file, and a symlinked package directory, which lint checks under its real
// path.
//
// The package holds only this file and the repository tests. They run the
// checks of internal/check against this repository: the platforms
// .goreleaser.yaml builds, the strict depguard rules of .golangci.yml, the
// packages go list reports for the layers and for the release binary, and
// every Go file under the root. The code lives in internal/check, so no
// package outside test/architecture can import it.
//
// Two of the tests hold the lint runs to the release platforms (issue #39).
// TestMakeLintRunsEveryReleaseTarget runs make lint with a fake golangci-lint
// and compares the runs it records with .goreleaser.yaml: one per release
// target with cgo off and one native run, each with a cache directory of its
// own and only the build settings make lint sets.
// TestCILintMatrixCoversEveryReleaseTarget reads the top level, the
// lint-target matrix and the Lint aggregator of .github/workflows/ci.yml as a
// closed schema, pins the golangci-lint step to make lint's build settings and
// the aggregator's script whole, and compares the matrix with .goreleaser.yaml.
//
// TestDepguardRulesArePinned pins every depguard rule of .golangci.yml, strict
// or not, against the copy pinnedDepguardRules: its list-mode, its files and
// allow lists and the pkg of each deny entry (issue #80). A merge conflict
// resolution once dropped the reporters-stay-offline rule and the exact
// jsondoc$ allow entry, and no check noticed. Adding, removing or changing a
// rule now means updating pinnedDepguardRules in the same commit, on purpose,
// where review sees it. TestDepguardPinCatchesMutations proves the pin
// notices, with 533 mutations of the real file: for every rule it deletes the
// rule and each of its files, allow and deny entries (every deny entry, a
// shared list's too), drops the trailing $ of each exact allow entry, adds an
// allow entry os and a negated files glob, and rewrites, deletes or adds the
// list-mode. A self-check fails if the walk misses an entry, and a document the
// reader refuses counts as caught only for the one mutation expected to break
// it: deleting app-uses-ports-not-adapters, which removes the &app-deny anchor
// a later alias reads.
//
// TestGolangciSettingsHideNoIssue covers what the pin does not: whether the
// rules run and what lint reports (issue #81). It fails, naming the setting, on
// linters.exclusions.paths or paths-except, a linters.exclusions.generated that
// is missing or not disable, an issues cap or filter, a run.issues-exit-code
// outside 1..125 (os.Exit truncates it to 8 bits, so 0, 256 or -256 exits 0),
// linters.default other than standard or all, run.tests not true, depguard or
// forbidigo missing from linters.enable or in linters.disable, and a
// linters.exclusions.rules entry naming depguard, naming forbidigo without a
// text or with an empty one, or naming no linter. The document's shape is an
// allow list that fails closed: version must be "2", and a key, a default or a
// preset the check does not know is refused. TestRepositoryReadsOnlyGolangciYML
// refuses a .golangci.json, .golangci.toml or .golangci.yaml at the root that
// git could commit, which golangci-lint would read in place of .golangci.yml.
// The golangci-lint command lines are pinned already, so neither can point
// lint at another config: TestMakeLintRunsEveryReleaseTarget wants exactly
// run --allow-serial-runners, and TestCILintMatrixCoversEveryReleaseTarget
// wants only version and skip-cache in the golangci-lint step's with: block.
//
// TestCloudAdapterSDKRuleIsChecked pins that the SDK-only rule,
// cloud-adapters-reach-the-cloud-through-the-sdk, is no skip (issue #79): the
// check reads its SDK-root prefix entries, the two directories it excludes and
// its test files, and TestCloudAdapterSDKRuleHoldsItsProbeForEveryReleaseTarget
// fails a fixture adapter that imports net/http on every release platform.
// TestCloudAdapterSDKRuleHoldsItsSDKProbeForEveryReleaseTarget passes a fixture
// adapter that imports a stand-in SDK: the SDK package and what it imports are
// third-party code an allowed import reaches, which depguard never lints, so
// the check exempts them, while a third-party package that a package no
// strict rule holds imports directly, or that no allow list reaches, is still
// reported. The exemption reads the strict allow lists only: the lax deny
// rules (go-cmp-only-in-tests, cloud-sdks-only-in-provider-adapters) are held
// by depguard in make lint alone. A third-party import in internal/cli is
// refused by depguard alone, whose strict rule there allows only $gostd and
// named module packages. One in cmd/eusov is refused by no rule but the lax
// SDK and go-cmp denies, so a new third-party dependency of the composition
// root is held only by review of the import and of go.mod: a known gap that a
// strict depguard rule for **/cmd/eusov/** would close, left to a follow-up
// issue. The exemption trusts the import path, which is sound because nothing
// can substitute other code for an allowed path: TestRootModuleHasNoReplace
// reads the root go.mod with the go command's own parser (go mod edit -json)
// and finds no replace, and TestRepositoryHasNoWorkspaceOrVendor finds no
// go.work, go.work.sum or vendor directory.
// TestOnlyTheSDKRuleHoldsPrefixEntries keeps prefix entries,
// which must end in /, to that rule.
//
// The checks are tests: they run in make test, make check and CI's make cover,
// with no workflow of their own. They need no network and no credentials.
package architecture
