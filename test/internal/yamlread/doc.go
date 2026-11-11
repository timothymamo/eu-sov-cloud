// Package yamlread reads the small part of YAML that .goreleaser.yaml,
// .golangci.yml and the workflows in .github/workflows use on the paths the
// repository's checks follow, with the standard library only (ADR 0012: the
// standard library has no YAML reader, and a new dependency needs a reason).
// test/architecture/internal/check, test/internal/releasetarget and test/lint
// read those files through it, so there is one reader to keep strict.
//
// It reads by indentation, line by line: a key's value is the text on its own
// line and the lines below it indented further. SplitLines cuts a document into
// content lines, Walk follows a path of keys, Mapping and Sequence read one
// block mapping or sequence, and List, Single, LiteralBlock and Scalar read a
// value. Along the path it walks and in the values it reads, it refuses
// everything else (anchors, aliases, merge keys, quoted or upper-case keys,
// tags, escapes, multi-line flow lists, folded or chomped block scalars, a
// repeated key) rather than guess at it; a literal block scalar (|) it reads
// only where a caller asks for one (LiteralBlock). The values of the keys it
// steps over are kept whole, by indentation, and never read. Anywhere in the
// document, comments included, it refuses the characters YAML readers do not
// agree on (CheckLine).
//
// The package is internal to test: it is not an API of the module. Its
// behaviour is pinned by the tests of the packages that read through it,
// test/internal/releasetarget's above all.
package yamlread
