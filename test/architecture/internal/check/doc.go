// Package check holds the package-level architecture checks that
// test/architecture runs against this repository (issue #40, ADR 0012).
//
// The checks work on packages, not file names. ListPackages lists packages and
// every package they depend on with go list, for one release platform, with
// cgo off or on and with a set of build tags, in an environment built from an
// allow list so that nothing outside its arguments changes the listing.
// Packages holds each package of the module to the allow list of every
// depguard rule whose directory covers it and does not exclude it, and, for a
// rule that holds test files, the imports of its _test.go files too (issue
// #79). An import is matched as depguard v2.2.1 matches it: the entries are
// sorted as written and only the one nearest before the import decides, exact
// for an entry ending in $, by string prefix for one ending in /. AllowList
// refuses a prefix entry that does not end in /, and a rule whose deny list
// names a path its allow list allows, since depguard then decides by entry
// length. A third-party package that in-module packages import only where a
// strict rule holds and allows the import, and everything it imports that no
// in-module package imports directly, is exempt: depguard lints no file of a
// dependency. Any other package outside the module that the layers reach is
// reported. The exemption reads the strict allow lists only: the lax deny
// rules (go-cmp-only-in-tests, cloud-sdks-only-in-provider-adapters) are held
// by depguard in make lint alone. No checked rule holds a third-party import
// in cmd/eusov or internal/cli: one in internal/cli is refused by depguard
// alone, whose strict rule allows only $gostd and named module packages, and
// one in cmd/eusov by no rule but the lax SDK and go-cmp denies, so a new
// third-party dependency of the composition root is held only by review of
// the import and of go.mod (a known gap; a strict depguard rule for
// **/cmd/eusov/** would close it). The exemption trusts the import path,
// which is sound because the root go.mod holds no replace directive, read
// with the go command's own parser (TestRootModuleHasNoReplace), and the
// repository holds no go.work, go.work.sum or vendor directory
// (TestRepositoryHasNoWorkspaceOrVendor), any of which could otherwise
// substitute code for an allowed import path. A package only a test imports
// is not listed by go list -deps, so it is not placed itself. Sources holds
// every package of the module, layer or not, to the rules that do not depend
// on a rule: it must belong to the main module, sit in the directory its
// import path names with symlinks resolved, and hold no cgo, C, assembly or
// other non-Go source.
// FindLineDirectives reports every line directive in a Go file git tracks or
// would add under a root, and every symlink among those files.
// StrayReleaseFiles and ReleaseWorkflow pin that the release builds what the
// check lists: no other goreleaser document, go.work or vendor at the root that
// git could commit, and a release workflow read as a closed schema of one
// release at a time and five jobs: the tests, behind the guard that the tagged commit is on main; a
// goreleaser job that runs no repository code and sets no environment for the
// go command; an image job that builds and pushes the image and can do nothing
// more; an sbom job that can only read the registry; and a sign job, the only
// one with an OIDC token, that signs and attests the image by digest with
// sigstore's and GitHub's actions alone. WorkflowPrivileges keeps every other
// workflow from publishing: no packages, id-token or attestations write, no
// write-all, no trigger a tag runs (push of tags, create, release,
// workflow_run), and a top-level permissions: always. DryRunMirrorsRelease
// holds ci.yml's image job, the release's dry run, to the release's build
// settings, tag rules, latest decision, SBOM settings, action pins and
// BuildKit image.
//
// GitHubRelease holds the release section of .goreleaser.yaml to exactly
// prerelease: auto, so a pre-release tag is a GitHub pre-release and never the
// latest release (issue #92).
//
// Nothing is copied: the release platforms are read from .goreleaser.yaml
// (ReleaseTargets), and the strict rules and their allow lists from
// .golangci.yml (StrictRules, RuleDirs, AllowList). These files and the
// workflows are read with the standard library, by a strict line reader keyed
// on indentation that refuses what it does not expect, and the characters YAML
// readers do not agree on, rather than guess at them.
//
// DepguardRules reads every depguard rule of .golangci.yml whole, and
// PinnedRules compares those rules with a pinned copy, naming the rule of every
// difference (issue #80).
//
// HidingSettings reads .golangci.yml for the settings that hide issues from
// make lint without touching a rule, and the ways of turning depguard or
// forbidigo off, and returns one message per setting found, naming it by its
// dotted key (issue #81). The document's shape is an allow list that fails
// closed: version must be "2", and a top-level, run, linters,
// linters.exclusions or issues key, a linters.default or a
// linters.exclusions.presets entry it does not know is refused. It wants
// linters.exclusions.generated: disable, which golangci-lint reads as lax when
// it is absent, and refuses a run.issues-exit-code outside 1..125, Go's
// portable exit range: os.Exit truncates the code to 8 bits, so 256 exits 0
// while lint prints issues. StrayLintConfigs refuses
// a .golangci.json, .golangci.toml or .golangci.yaml at the root that git
// could commit, any of which golangci-lint would read in place of
// .golangci.yml.
//
// The package is internal to test/architecture: it is not an API of the module.
package check
