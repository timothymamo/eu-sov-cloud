package architecture_test

import (
	"bytes"
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
	"github.com/timothymamo/shift2eu/test/internal/openapibundle"
)

// TestReleasePublishesTheBundledDocument pins one source for the bundle's path
// (the review of issue #139). test/openapibundle writes the document
// openapibundle.Path names, and the release publishes exactly that file, so
// check.BundleGlob must be "./" + openapibundle.Path, .goreleaser.yaml must
// hold that glob wherever it names the bundle, and both GitHubRelease and
// GoReleaserArchives must refuse a document whose glob is anything else.
//
// Before this test the bundler's path (test/openapibundle's own constant) and
// the release's glob (check's bundleGlob) were two copies of the same string,
// held together by convention alone: a release that published a file the
// bundler never writes would ship a stale document, and no test noticed.
func TestReleasePublishesTheBundledDocument(t *testing.T) {
	t.Parallel()

	// The one glob the release may name: the bundler's path, as .goreleaser
	// writes a repository-relative file, "./" first.
	glob := "./" + openapibundle.Path

	if check.BundleGlob != glob {
		t.Errorf("check.BundleGlob is %q, want %q: the glob the release requires is the path the bundler writes", check.BundleGlob, glob)
	}

	goreleaser := readRepositoryFile(t, ".goreleaser.yaml")

	if !bytes.Contains(goreleaser, []byte(glob)) {
		t.Fatalf(".goreleaser.yaml holds no %q, want the release to publish %s, the document the bundler writes", glob, openapibundle.Path)
	}

	// Both checks require exactly that glob: the same document with another
	// one is refused, so neither can be satisfied by a path the bundler never
	// writes.
	mutated := bytes.ReplaceAll(goreleaser, []byte(glob), []byte(glob+".old"))

	if err := check.GitHubRelease(mutated); err == nil {
		t.Errorf("GitHubRelease() of .goreleaser.yaml with the glob %q = nil, want an error: the release publishes the bundle as %q", glob+".old", glob)
	}

	if messages := check.GoReleaserArchives(mutated); len(messages) == 0 {
		t.Errorf("GoReleaserArchives() of .goreleaser.yaml with the glob %q = no messages, want one: checksums.txt covers the bundle at %q", glob+".old", glob)
	}
}
