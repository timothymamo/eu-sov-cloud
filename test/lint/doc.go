// Package lint holds rules on the repository's Go source that golangci-lint
// cannot express, written as plain tests, and the probe that proves a
// golangci-lint rule still fires. make lint runs them after golangci-lint, and
// as they carry no build tag, make test, make check and make cover (CI's Test
// job) run them too. The package has no code of its own.
//
// TestSourcesHideNothingFromDepguard reads every Go file the go command's
// ./... pattern reaches, whatever its build constraints, so every file make
// lint lints in any of its golangci-lint runs (native and one per release
// target), and fails on each way a file could hide code from depguard, which
// enforces the dependency rule (docs/architecture.md), or from forbidigo,
// which enforces the cloud-read-only rule (CLAUDE.md rule 1):
//
//   - A nolint directive that would hide a depguard or a forbidigo issue: one
//     that names depguard or forbidigo, one that names all, and one that names
//     no linter. nolintlint, configured in .golangci.yml, requires a directive
//     to name its linters and give a reason, but it cannot refuse one named
//     linter, and it lets some directives that silence every linter through,
//     such as //nolint: all with a space after the colon.
//   - A //go:build or // +build line that negates the e2e or integration tag.
//     Every lint run sets both, so lint would never read a file behind !e2e
//     that a build without the tag compiles.
//   - A Go file that is a symbolic link: golangci-lint lints it with the
//     directives of its target, wherever that lies.
//
// The negated tags are those of run.build-tags in .golangci.yml, which
// TestNegatableTagsMatchGolangci keeps in step; the go command runs below take
// their -tags from the same list. test/architecture refuses the other ways:
// TestRepositoryHasNoLineDirectives fails on a //line directive, whose
// position depguard's file globs would match, and on any symlink git tracks or
// would add. Refusing symlinked Go files here too is deliberate defence in
// depth: this walk does not depend on git and also reaches files git ignores.
// Every test here that reads Go files, of the module or of a probe, reads them
// through this one walk, walkGoFiles, so each refuses a symlinked Go file.
// docs/architecture.md lists every way and its check in one place.
//
// TestCloudReadOnlyProbe runs the golangci-lint version CI pins, forbidigo and
// depguard, on probe source adapters over stub cloud SDKs
// (testdata/cloudreadonly), offline and with no credentials. Each probe line
// that ends in a want comment must be refused with the messages it names, no
// other line may be, and every entry of forbidigo's cloud-read-only group must
// refuse a line of its own, so a pattern that silently stops matching fails.
// The harness, runForbidigoProbe, serves any forbidigo group. Where
// golangci-lint is not on PATH (CI's Test job) it skips, unless
// EUSOV_LINT_PROBES=require, which make lint and every entry of CI's
// lint-target matrix (the Lint check) set. Only CI fails on a golangci-lint
// version other than the pinned one; elsewhere the probe runs and logs a
// warning. TestCloudReadOnlyPatterns and
// TestCloudReadOnlyPackages check the group's allowlist patterns, read from
// .golangci.yml, against lists of write and read names and of import paths,
// without golangci-lint. TestForbidigoExclusionsDropOneGroup checks every
// exclusion rule that drops forbidigo issues: it names forbidigo alone, its
// text matches only one group's quoted msg, and the rules of each group are
// exactly those it states: none for cloud-read-only, and for domain-is-pure
// one path and one path-except. It reads linters.exclusions.rules with
// test/internal/yamlread, the strict line reader test/architecture reads
// .golangci.yml with, so a rule written in a way it does not follow fails.
//
// TestLintLoadsEveryGoFile and TestLintVisitsEveryImportedPackage keep every
// Go file within lint's reach. Lint loads, for the host and every release
// target of .goreleaser.yaml, what builds with the tags e2e and integration,
// and ./... skips testdata, _ and . directories, so a file behind another build
// tag, or an imported package in a skipped directory, would run unchecked, with
// real credentials in an integration test. The tests run the go command
// offline. TestLintLoadsEveryGoFile reads the release targets from
// .goreleaser.yaml with test/internal/releasetarget, the reader
// test/architecture holds the release to, and counts a file as loaded when the
// go command loads it for any of them with cgo off and the baseline GOAMD64
// and GOARM64 levels, as the release builds them; a file only for one release
// platform passes, and a platform the release gains is one the test holds
// files to. A file only a cgo build compiles is refused on purpose: the
// release builds with CGO_ENABLED=0, and only the release targets count
// toward loaded, not make lint's native run with cgo on, which the release
// never ships. TestUnloadedGoFilesProbe pins
// its helper on a probe module (testdata/platformprobe).
//
// TestNoInterfaceDeclaresACloudWrite and TestNoTypeHidesACloudSDKType close
// what forbidigo's cloud-read-only group cannot see. An interface a cloud
// source adapter declares, generic constraints and interface literals
// included, may declare only read verbs or unexported methods, and an interface
// elsewhere no method named for a write; no struct embeds an SDK type.
// TestInterfaceFindingsProbe runs the interface rule on the probe, and
// TestWriteVerbsMatchForbidigo keeps the write verbs the same here and in
// .golangci.yml.
package lint
