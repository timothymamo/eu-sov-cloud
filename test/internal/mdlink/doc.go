// Package mdlink checks the relative links in the repository's Markdown, with
// the standard library only: test/docs runs it over README.md, docs/ and the
// skills, so a moved page or a renamed heading fails make check rather than a
// reader.
package mdlink
