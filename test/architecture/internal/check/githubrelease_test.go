package check_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// TestGitHubRelease pins how the GitHub release is published: release holds
// exactly prerelease: auto, so a tag with a pre-release suffix (v0.1.0-rc.2)
// is a GitHub pre-release and never the latest release (issue #92), and every
// other document is refused.
func TestGitHubRelease(t *testing.T) {
	t.Parallel()

	buildFlow := goreleaserFixture(t, "flow")
	buildReal := goreleaserFixture(t, "real")

	tests := []struct {
		name       string
		goreleaser string
		wantErr    bool
	}{
		{
			name:       "prerelease auto is accepted",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: auto\n",
		},
		{
			name:       "a quoted auto is accepted",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: \"auto\"\n",
		},
		{
			name:       "the real document with its release section is accepted",
			goreleaser: buildReal + "\nrelease:\n  prerelease: auto\n",
		},
		{
			name:       "CRLF line ends are accepted",
			goreleaser: "version: 2\r\n\r\nrelease:\r\n  prerelease: auto\r\n",
		},
		{
			name:       "a comment after the value is accepted",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: auto # rc tags\n",
		},
		{
			name:       "no release section is refused: goreleaser's default publishes every tag as a normal release",
			goreleaser: buildFlow,
			wantErr:    true,
		},
		{
			name:       "an empty release section is refused",
			goreleaser: buildFlow + "\nrelease:\n",
			wantErr:    true,
		},
		{
			name:       "a release section without prerelease is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft: false\n",
			wantErr:    true,
		},
		{
			name:       "prerelease false is refused",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: false\n",
			wantErr:    true,
		},
		{
			name:       "prerelease true is refused",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: true\n",
			wantErr:    true,
		},
		{
			name:       "an upper-case Auto is refused",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: Auto\n",
			wantErr:    true,
		},
		{
			name:       "a templated prerelease is refused",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: '{{ .Env.PRERELEASE }}'\n",
			wantErr:    true,
		},
		{
			name:       "an empty prerelease is refused",
			goreleaser: buildFlow + "\nrelease:\n  prerelease:\n",
			wantErr:    true,
		},
		{
			name:       "an anchored auto is refused",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: &a auto\n",
			wantErr:    true,
		},
		{
			// make_latest would override the latest decision prerelease: auto
			// makes, putting a release candidate back on top.
			name:       "make_latest beside prerelease is refused",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: auto\n  make_latest: true\n",
			wantErr:    true,
		},
		{
			name:       "any other release key beside prerelease is refused",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: auto\n  mode: replace\n",
			wantErr:    true,
		},
		{
			name:       "a flow mapping release is refused",
			goreleaser: buildFlow + "\nrelease: {prerelease: auto}\n",
			wantErr:    true,
		},
		{
			name:       "a repeated release key is refused",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: auto\nrelease:\n  prerelease: false\n",
			wantErr:    true,
		},
		{
			name:       "a prerelease key outside release is not read as the setting",
			goreleaser: buildFlow + "\nchangelog:\n  prerelease: auto\n",
			wantErr:    true,
		},
		{
			name:       "an empty document is refused",
			goreleaser: "",
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := check.GitHubRelease([]byte(tc.goreleaser))
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Fatalf("GitHubRelease() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// goreleaserFixture returns the .goreleaser.yaml document name of
// test/internal/releasetarget/testdata, a snapshot of the shape of this
// repository's .goreleaser.yaml (before hooks, archives, checksum and
// changelog around one build) that releasetarget.Read accepts, so a case here
// only adds a release section to a build the release-target reader reads.
// test/architecture/repository_test.go checks the real .goreleaser.yaml itself.
func goreleaserFixture(t *testing.T, name string) string {
	t.Helper()

	doc, err := os.ReadFile(filepath.Join("..", "..", "..", "internal", "releasetarget", "testdata", name+".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	return string(doc)
}
