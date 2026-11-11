// Package architecture holds the architecture checks that depguard cannot make
// (issue #40, ADR 0011). depguard reads the imports of the Go files
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
// The checks are tests: they run in make test, make check and CI's make cover,
// with no workflow of their own. They need no network and no credentials.
package architecture
