// Package lint holds rules on the repository's Go source that golangci-lint
// cannot express, written as plain tests. They carry no build tag, so make
// test, make check and make cover (CI's Test job) run them. The package has no
// code of its own.
//
// TestNoNolintSilencesDepguard refuses every nolint directive that would hide a
// depguard issue, which is how the dependency rule is enforced
// (docs/architecture.md): a directive that names depguard, one that names all,
// and one that names no linter. nolintlint, configured in .golangci.yml,
// requires a directive to name its linters and give a reason, but it cannot
// refuse one named linter, and it lets some directives that silence every
// linter through, such as //nolint: all with a space after the colon.
package lint
