// Package docs checks the repository's documentation as plain tests, without
// the network: every relative link and #anchor in README.md, docs/**/*.md and
// .claude/skills/**/*.md resolves to a file and a heading that exist, and every
// page under docs/ and README.md starts with the single # heading that is its
// title (docs/README.md, "Writing a page"). External URLs are not fetched.
// make test, and so make check, run it. The package has no code of its own; the
// checks are in test/internal/mdlink.
package docs
