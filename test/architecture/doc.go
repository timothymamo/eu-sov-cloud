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
// The checks are tests: they run in make test, make check and CI's make cover,
// with no workflow of their own. They need no network and no credentials.
package architecture
