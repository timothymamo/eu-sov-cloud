// Package check holds the package-level architecture checks that
// test/architecture runs against this repository (issue #40, ADR 0012).
//
// The checks work on packages, not file names. ListPackages lists packages and
// every package they depend on with go list, for one release platform, with
// cgo off or on and with a set of build tags, in an environment built from an
// allow list so that nothing outside its arguments changes the listing.
// Packages holds each package of the module to the allow list of every
// depguard rule whose directory covers it; Sources holds every package of the
// module, layer or not, to the rules that do not depend on a rule: it must
// belong to the main module, sit in the directory its import path names with
// symlinks resolved, and hold no cgo, C, assembly or other non-Go source.
// FindLineDirectives reports every line directive in a Go file git tracks or
// would add under a root, and every symlink among those files.
// StrayReleaseFiles and ReleaseWorkflow pin that the release builds what the
// check lists: no other goreleaser document, go.work or vendor at the root that
// git could commit, and a release workflow read as a closed schema of three
// jobs: the tests, a goreleaser job that runs no repository code and sets no
// environment for the go command, and an image job that publishes only the
// signed image, its provenance and its SBOM, by digest, with the write
// permissions split between the two.
//
// Nothing is copied: the release platforms are read from .goreleaser.yaml
// (ReleaseTargets), and the strict rules and their allow lists from
// .golangci.yml (StrictRules, RuleDirs, AllowList). Both files are read with
// the standard library, by a strict line reader keyed on indentation that
// refuses what it does not expect, and the characters YAML readers do not
// agree on, rather than guess at them.
//
// The package is internal to test/architecture: it is not an API of the module.
package check
