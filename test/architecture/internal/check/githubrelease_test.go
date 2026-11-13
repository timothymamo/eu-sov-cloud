package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// bundleExtraFile is the one extra file the release publishes beside the
// archives (issue #139): the bundled OpenAPI document, under the name its
// release asset has. The checksum section holds the same entry, so
// checksums.txt covers it (TestGoReleaserArchives).
const bundleExtraFile = `  extra_files:
    - glob: ./api/openapi.bundled.json
      name_template: shift2eu-server_openapi.json
`

// releaseSection returns the release: section of .goreleaser.yaml for the
// given prerelease value, with draft: true, replace_existing_draft: true and
// the bundle's extra file.
func releaseSection(prerelease string) string {
	return "release:\n  draft: true\n  prerelease: " + prerelease + "\n  replace_existing_draft: true\n" + bundleExtraFile
}

// TestGitHubRelease pins how the GitHub release is published: release holds
// exactly draft: true, prerelease: auto and replace_existing_draft: true, in
// that order, and exactly the one extra file the release publishes beside the
// archives, the bundled OpenAPI document api/openapi.bundled.json as
// shift2eu-server_openapi.json (issue #139). Every asset GoReleaser uploads
// goes to a draft release, which locks its assets when it is published (issue
// #156, ADR 0015), so a job of release.yml publishes it last;
// replace_existing_draft: true makes a re-run of a failed goreleaser job
// replace the draft that job left, rather than add a second one for the tag
// (GoReleaser finds a draft release by neither GetReleaseByTag nor a tag
// lookup); and a tag with a pre-release suffix (v0.1.0-rc.2) is a GitHub
// pre-release and never the latest release (issue #92). Every other document
// is refused.
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
			name:       "draft, prerelease auto and replace_existing_draft are accepted",
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n  prerelease: auto\n  replace_existing_draft: true\n" + bundleExtraFile,
		},
		{
			name:       "a quoted auto is accepted",
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n  prerelease: \"auto\"\n  replace_existing_draft: true\n" + bundleExtraFile,
		},
		{
			name:       "the real document with its release section is accepted",
			goreleaser: buildReal + "\nrelease:\n  draft: true\n  prerelease: auto\n  replace_existing_draft: true\n" + bundleExtraFile,
		},
		{
			name:       "CRLF line ends are accepted",
			goreleaser: "version: 2\r\n\r\nrelease:\r\n  draft: true\r\n  prerelease: auto\r\n  replace_existing_draft: true\r\n" + strings.ReplaceAll(bundleExtraFile, "\n", "\r\n"),
		},
		{
			name:       "a comment after the value is accepted",
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n  prerelease: auto # rc tags\n  replace_existing_draft: true # a failed run's draft is replaced\n" + bundleExtraFile,
		},
		{
			// A re-run of a failed goreleaser job leaves the draft the failed
			// run created. GoReleaser finds an existing release by its tag with
			// GetReleaseByTag, which skips drafts, so without this key it
			// creates a second draft for the tag instead of replacing the first.
			name:       "a release section without replace_existing_draft is refused: a re-run would leave two drafts for the tag",
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n  prerelease: auto\n" + bundleExtraFile,
			wantErr:    true,
		},
		{
			name:       "replace_existing_draft false is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n  prerelease: auto\n  replace_existing_draft: false\n" + bundleExtraFile,
			wantErr:    true,
		},
		{
			name:       "a quoted replace_existing_draft is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n  prerelease: auto\n  replace_existing_draft: \"true\"\n" + bundleExtraFile,
			wantErr:    true,
		},
		{
			name:       "replace_existing_draft before prerelease is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n  replace_existing_draft: true\n  prerelease: auto\n" + bundleExtraFile,
			wantErr:    true,
		},
		{
			// A published release's assets cannot be replaced, so goreleaser
			// must upload them to a draft and publish it last (issue #156). Its
			// default publishes the release, assets and all, at once.
			name:       "a release section without draft is refused: goreleaser's default publishes the assets it uploads",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: auto\n  replace_existing_draft: true\n" + bundleExtraFile,
			wantErr:    true,
		},
		{
			name:       "draft false is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft: false\n  prerelease: auto\n" + bundleExtraFile,
			wantErr:    true,
		},
		{
			name:       "an upper-case True draft is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft: True\n  prerelease: auto\n" + bundleExtraFile,
			wantErr:    true,
		},
		{
			name:       "an empty draft is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft:\n  prerelease: auto\n" + bundleExtraFile,
			wantErr:    true,
		},
		{
			name:       "a draft from an expression is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft: '{{ .Env.DRAFT }}'\n  prerelease: auto\n" + bundleExtraFile,
			wantErr:    true,
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
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n",
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
			name:       "make_latest beside prerelease is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n  prerelease: auto\n  replace_existing_draft: true\n  make_latest: true\n",
			wantErr:    true,
		},
		{
			name:       "any other release key beside draft, prerelease and replace_existing_draft is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n  prerelease: auto\n  replace_existing_draft: true\n  mode: replace\n",
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

		// The bundle the release publishes beside the archives (issue #139):
		// release: holds exactly one extra file, the bundled OpenAPI document
		// under the name its asset has, so the release ships it. The
		// checksum section's copy of the entry is TestGoReleaserArchives's.
		{
			name:       "a release with no extra file is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n  prerelease: auto\n  replace_existing_draft: true\n",
			wantErr:    true,
		},
		{
			name:       "an empty extra_files is refused",
			goreleaser: buildFlow + "\nrelease:\n  draft: true\n  prerelease: auto\n  replace_existing_draft: true\n  extra_files:\n",
			wantErr:    true,
		},
		{
			name: "an extra file with another glob is refused",
			goreleaser: buildFlow + "\n" +
				mutate(t, releaseSection("auto"), "./api/openapi.bundled.json", "./api/openapi.json"),
			wantErr: true,
		},
		{
			name: "an extra file with another name_template is refused",
			goreleaser: buildFlow + "\n" +
				mutate(t, releaseSection("auto"), "shift2eu-server_openapi.json", "openapi.json"),
			wantErr: true,
		},
		{
			// Two extra files: the release publishes one, and the second is one
			// nothing pins.
			name:       "two extra files are refused",
			goreleaser: buildFlow + "\n" + releaseSection("auto") + "    - glob: ./api/openapi.json\n      name_template: openapi.json\n",
			wantErr:    true,
		},
		{
			name: "an extra file with a key the check never lists is refused",
			goreleaser: buildFlow + "\n" +
				mutate(t, releaseSection("auto"), "      name_template: shift2eu-server_openapi.json\n", "      name_template: shift2eu-server_openapi.json\n      strip_parent: true\n"),
			wantErr: true,
		},
		{
			name: "an extra file that is no mapping is refused",
			goreleaser: buildFlow + "\n" +
				mutate(t, releaseSection("auto"), "  extra_files:\n    - glob: ./api/openapi.bundled.json\n      name_template: shift2eu-server_openapi.json\n", "  extra_files: [./api/openapi.bundled.json]\n"),
			wantErr: true,
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
