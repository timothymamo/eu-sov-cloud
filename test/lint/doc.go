// Package lint holds rules on the repository's Go source that golangci-lint
// cannot express, written as plain tests. make lint runs them after
// golangci-lint, and as they carry no build tag, make test, make check and make
// cover (CI's Test job) run them too. The package has no code of its own.
//
// TestSourcesHideNothingFromDepguard reads every Go file the go command's
// ./... pattern reaches, whatever its build constraints, so every file make
// lint lints in any of its golangci-lint runs (native and one per release
// target), and fails on each way a file could hide code from depguard, which
// enforces the dependency rule (docs/architecture.md):
//
//   - A nolint directive that would hide a depguard issue: one that names
//     depguard, one that names all, and one that names no linter. nolintlint,
//     configured in .golangci.yml, requires a directive to name its linters
//     and give a reason, but it cannot refuse one named linter, and it lets
//     some directives that silence every linter through, such as //nolint: all
//     with a space after the colon.
//   - A //go:build or // +build line that negates the e2e or integration tag.
//     Every lint run sets both, so lint would never read a file behind !e2e
//     that a build without the tag compiles.
//   - A Go file that is a symbolic link: golangci-lint lints it with the
//     directives of its target, wherever that lies.
//
// The negated tags are those of run.build-tags in .golangci.yml, which
// TestNegatableTagsMatchGolangci keeps in step. test/architecture refuses the
// other ways: TestRepositoryHasNoLineDirectives fails on a //line directive,
// whose position depguard's file globs would match, and on any symlink git
// tracks or would add. Refusing symlinked Go files here too is deliberate
// defence in depth: this walk does not depend on git and also reaches files
// git ignores. docs/architecture.md lists every way and its check in one
// place.
package lint
