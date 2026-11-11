package check_test

import (
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// TestGitHubRelease pins how the GitHub release is published: release holds
// exactly prerelease: auto, so a tag with a pre-release suffix (v0.1.0-rc.2)
// is a GitHub pre-release and never the latest release (issue #92), and every
// other document is refused.
func TestGitHubRelease(t *testing.T) {
	t.Parallel()

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

// The .goreleaser.yaml documents below are copies of two in
// test/internal/releasetarget, kept here so each case adds a release section to
// a build releasetarget.Read accepts.
// reads, kept here so each case adds a release section to a build ReleaseTarget accepts.
const (
	// buildFlow writes the goos and goarch lists in flow form, as
	// .goreleaser.yaml does.
	buildFlow = `version: 2

project_name: eusov

builds:
  - id: eusov
    main: ./cmd/eusov
    binary: eusov
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]

archives:
  - formats: [tar.gz]
`

	// buildReal is this repository's .goreleaser.yaml in the shape the release
	// document really has: before hooks, archives, checksum and changelog
	// around one build. The check may refuse a top-level key that could change
	// what the release builds, so the document the release actually builds
	// from must stay accepted, character for character.
	buildReal = `version: 2

project_name: eusov

before:
  hooks:
    - go mod tidy

builds:
  - id: eusov
    main: ./cmd/eusov
    binary: eusov
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w
      - -X github.com/timothymamo/eu-sov-cloud/internal/version.version={{ .Version }}
      - -X github.com/timothymamo/eu-sov-cloud/internal/version.commit={{ .ShortCommit }}
      - -X github.com/timothymamo/eu-sov-cloud/internal/version.date={{ .Date }}
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]

archives:
  - formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]

checksum:
  name_template: checksums.txt

changelog:
  use: github
`
)
