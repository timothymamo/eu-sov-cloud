package check_test

import (
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// The fixture below is .goreleaser.yaml in parts, as the release leaves it
// after issue #147: the top level with the two builds, then one archive entry
// per binary, then the checksum section. Each archive entry is a constant of
// its own, so a case changes one entry and leaves the other exactly as the
// release has it.

// goreleaserArchivesTop is .goreleaser.yaml up to its archives:, with the two
// builds the release makes: the CLI's (shift2eu) and the HTTP API's
// (shift2eu-server).
const goreleaserArchivesTop = `version: 2

project_name: shift2eu

builds:
  - id: shift2eu
    main: ./cmd/shift2eu
    binary: shift2eu
  - id: shift2eu-server
    main: ./cmd/shift2eu-server
    binary: shift2eu-server

archives:
`

// cliArchive is the archive entry of the CLI's build: its own id, the builds
// it holds (shift2eu alone), the name the CLI's archives have always had,
// goreleaser's default template written out, tar.gz on every platform, zip on
// Windows, and the files the archive ships beside the binary (LICENSE and
// README.md, with the pages under docs/ that README.md links to, issue #149).
const cliArchive = `  - id: shift2eu
    ids: [shift2eu]
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - LICENSE
      - README.md
      - docs/*.md
      - docs/**/*

`

// serverArchive is the archive entry of the HTTP API's build: its own id, the
// build it holds (shift2eu-server alone), its own name,
// shift2eu-server_<version>_<os>_<arch>, the same formats and the same files
// as the CLI's.
const serverArchive = `  - id: shift2eu-server
    ids: [shift2eu-server]
    name_template: "{{ .ProjectName }}-server_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - LICENSE
      - README.md
      - docs/*.md
      - docs/**/*

`

// goreleaserChecksum ends the document: one checksums.txt covering every
// archive of the release.
const goreleaserChecksum = `checksum:
  name_template: checksums.txt
`

// goreleaserArchives renders .goreleaser.yaml from its parts: the top level,
// the CLI's archive, the server's archive and the checksum section.
func goreleaserArchives(cli, server string) string {
	return goreleaserArchivesTop + cli + server + goreleaserChecksum
}

// TestGoReleaserArchives pins issue #147: the release ships shift2eu and
// shift2eu-server as two archives of their own, under the same tag and
// version. check.GoReleaserArchives reads .goreleaser.yaml and returns one
// message per way the document lets one archive hold both binaries, or one
// binary move out of the archive its downloads expect:
//
//   - archives: holds exactly two entries, and each names exactly one build
//     through ids: an entry without ids takes every build, and every build of
//     builds: sits in exactly one archive: the CLI's, the build whose binary
//     is shift2eu, and the server's, whose binary is shift2eu-server, each
//     with the ids of its own build alone. The message names the build that
//     is in no archive, or the id that names no build;
//   - each entry's id: is the one build its ids: names, and no two entries
//     share one: goreleaser defaults an entry's id to "default", so two
//     entries that leave it out are both the archive "default" to it and
//     `goreleaser check` refuses the document ("found 2 archives with the ID
//     'default'"). An entry with no id:, two entries with the same id:, and
//     an id: that is not the build its ids: names are refused, worded with
//     id;
//   - each entry's name_template is the one that build's downloads expect:
//     {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }} for the CLI,
//     whose archive names stay exactly as they were, and
//     {{ .ProjectName }}-server_{{ .Version }}_{{ .Os }}_{{ .Arch }} for the
//     server, so that one is shift2eu-server_<version>_<os>_<arch>. A
//     name_template that is missing or differs is refused, worded with
//     name_template;
//   - each entry is tar.gz on every platform and zip on Windows (formats: and
//     format_overrides:, worded with tar.gz and zip);
//   - each entry ships exactly LICENSE, README.md, docs/*.md and docs/**/*
//     (files:), worded with the entry it does not ship;
//   - the checksum section holds exactly name_template: checksums.txt, so one
//     checksums.txt covers every archive: a missing section, another name and
//     any other key in it (split: for one) are refused, worded with what it
//     holds instead;
//   - an archive entry holds no key beyond id, ids, name_template, formats,
//     format_overrides and files: the section is a closed schema, so a key
//     the check never lists cannot change what an archive is.
//
// An accepted document returns no message at all.
func TestGoReleaserArchives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		goreleaser string
		// wantIn is a part of the message the check must return; "" means
		// none at all.
		wantIn string
	}{
		// The accepted shape.
		{name: "two archives, one per binary", goreleaser: goreleaserArchives(cliArchive, serverArchive)},

		// The document's line ends do not matter, as in every reader here.
		{
			name:       "the accepted document with CRLF line ends",
			goreleaser: strings.ReplaceAll(goreleaserArchives(cliArchive, serverArchive), "\n", "\r\n"),
		},

		// The archives section itself.
		{
			name:       "no archives section",
			goreleaser: strings.TrimSuffix(goreleaserArchivesTop, "archives:\n"),
			wantIn:     "archives",
		},
		{
			name:       "one archive for two builds",
			goreleaser: goreleaserArchives(cliArchive, ""),
			wantIn:     "shift2eu-server",
		},
		{
			// An entry without ids takes every build, so the download would
			// hold both binaries again.
			name: "an archive entry with no ids",
			goreleaser: goreleaserArchives(
				mutate(t, cliArchive, "    ids: [shift2eu]\n    name_template:", "    name_template:"),
				serverArchive,
			),
			wantIn: "ids",
		},
		{
			name: "an archive whose ids names both builds",
			goreleaser: goreleaserArchives(
				cliArchive,
				mutate(t, serverArchive, "ids: [shift2eu-server]", "ids: [shift2eu-server, shift2eu]"),
			),
			wantIn: "ids",
		},
		{
			// Both entries take the CLI's build, so the server is in no
			// archive at all.
			name: "both archives naming the CLI's build",
			goreleaser: goreleaserArchives(
				cliArchive,
				mutate(t, serverArchive, "ids: [shift2eu-server]", "ids: [shift2eu]"),
			),
			wantIn: "shift2eu-server",
		},
		{
			name: "an archive whose ids names no build",
			goreleaser: goreleaserArchives(
				cliArchive,
				mutate(t, serverArchive, "ids: [shift2eu-server]", "ids: [shift2eu-http]"),
			),
			wantIn: "shift2eu-http",
		},

		// The archive ids: goreleaser defaults an entry's id to "default", so
		// two entries that leave it out are the same archive to it and
		// `goreleaser check` refuses the document. The rule is one id per
		// entry, equal to the one build its ids names, so each case below
		// pins that the message names that id: the one the entry wants
		// (shift2eu) or the one it wrongly carries (shift2eu-api). Wording the
		// message with the id is what tells the reader which archive to fix.
		{
			name: "an archive entry with no id",
			goreleaser: goreleaserArchives(
				mutate(t, cliArchive, "  - id: shift2eu\n    ids: [shift2eu]\n", "  - ids: [shift2eu]\n"),
				serverArchive,
			),
			wantIn: "shift2eu",
		},
		{
			name: "two archive entries with the same id",
			goreleaser: goreleaserArchives(
				cliArchive,
				mutate(t, serverArchive, "  - id: shift2eu-server\n", "  - id: shift2eu\n"),
			),
			wantIn: "shift2eu",
		},
		{
			// The id of an entry is the one build its ids names: here the
			// server's entry holds shift2eu-server but is called
			// shift2eu-api, an id no build has.
			name: "an archive whose id differs from its single ids value",
			goreleaser: goreleaserArchives(
				cliArchive,
				mutate(t, serverArchive, "  - id: shift2eu-server\n", "  - id: shift2eu-api\n"),
			),
			wantIn: "shift2eu-api",
		},

		// The archive names: the CLI's must not change, and the server's must
		// be its own.
		{
			name: "a CLI name_template that drifts",
			goreleaser: goreleaserArchives(
				mutate(t, cliArchive, `"{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`, `"{{ .ProjectName }}_{{ .Version }}_{{ .Os }}"`),
				serverArchive,
			),
			wantIn: "name_template",
		},
		{
			name: "a CLI name_template that is no template",
			goreleaser: goreleaserArchives(
				mutate(t, cliArchive, `"{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`, `"shift2eu"`),
				serverArchive,
			),
			wantIn: "name_template",
		},
		{
			// The CLI's template on both entries would name the server's
			// archive shift2eu_<version>_<os>_<arch> as well, and the two
			// would collide.
			name: "the server's name_template is the CLI's",
			goreleaser: goreleaserArchives(
				cliArchive,
				mutate(t, serverArchive, `"{{ .ProjectName }}-server_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`, `"{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`),
			),
			wantIn: "name_template",
		},
		{
			name: "a server archive with no name_template",
			goreleaser: goreleaserArchives(
				cliArchive,
				mutate(t, serverArchive, "    name_template: \"{{ .ProjectName }}-server_{{ .Version }}_{{ .Os }}_{{ .Arch }}\"\n", ""),
			),
			wantIn: "name_template",
		},

		// The formats: tar.gz everywhere, zip on Windows, in each entry.
		{
			name: "an archive that is not tar.gz",
			goreleaser: goreleaserArchives(
				mutate(t, cliArchive, "    formats: [tar.gz]", "    formats: [zip]"),
				serverArchive,
			),
			wantIn: "tar.gz",
		},
		{
			name: "a Windows archive that is not zip",
			goreleaser: goreleaserArchives(
				cliArchive,
				mutate(t, serverArchive, "    format_overrides:\n      - goos: windows\n        formats: [zip]\n", ""),
			),
			wantIn: "zip",
		},

		// The files each archive ships beside its binary (issue #149).
		{
			name: "an archive that ships no LICENSE",
			goreleaser: goreleaserArchives(
				mutate(t, cliArchive, "      - LICENSE\n", ""),
				serverArchive,
			),
			wantIn: "LICENSE",
		},
		{
			name: "an archive that ships no README.md",
			goreleaser: goreleaserArchives(
				cliArchive,
				mutate(t, serverArchive, "      - README.md\n", ""),
			),
			wantIn: "README.md",
		},
		{
			name: "an archive without the pages directly in docs/",
			goreleaser: goreleaserArchives(
				cliArchive,
				mutate(t, serverArchive, "      - docs/*.md\n", ""),
			),
			wantIn: "docs/*.md",
		},
		{
			name: "an archive without the pages in the docs/ subdirectories",
			goreleaser: goreleaserArchives(
				mutate(t, cliArchive, "      - docs/**/*\n", ""),
				serverArchive,
			),
			wantIn: "docs/**/*",
		},

		// The checksum: one file covering every archive.
		{
			name:       "no checksum section",
			goreleaser: goreleaserArchivesTop + cliArchive + serverArchive,
			wantIn:     "checksum",
		},
		{
			name: "a checksum name that is not checksums.txt",
			goreleaser: goreleaserArchivesTop + cliArchive + serverArchive +
				"checksum:\n  name_template: SHA256SUMS.txt\n",
			wantIn: "checksums.txt",
		},
		{
			// split: true publishes one checksums file per archive, so no
			// single checksums.txt would cover the release.
			name: "a checksum section that splits the file",
			goreleaser: goreleaserArchivesTop + cliArchive + serverArchive +
				"checksum:\n  name_template: checksums.txt\n  split: true\n",
			wantIn: "split",
		},

		// The closed schema.
		{
			name: "an archive entry with a key the check never lists",
			goreleaser: goreleaserArchives(
				mutate(t, cliArchive, "    formats: [tar.gz]\n", "    formats: [tar.gz]\n    wrap_in_directory: true\n"),
				serverArchive,
			),
			wantIn: "wrap_in_directory",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := check.GoReleaserArchives([]byte(tc.goreleaser))
			joined := strings.Join(got, "\n")

			if tc.wantIn == "" {
				if len(got) != 0 {
					t.Errorf("GoReleaserArchives() = %q, want no message", got)
				}

				return
			}

			if !strings.Contains(joined, tc.wantIn) {
				t.Errorf("GoReleaserArchives() = %q, want a message containing %q", got, tc.wantIn)
			}
		})
	}
}

// ciGoreleaserInstall is the step of .github/workflows/ci.yml's test job that
// installs the GoReleaser the release runs: the same action commit, the same
// distribution and the same version as release.yml's goreleaser step, installed
// only, so the snapshot that builds the archives in the dry run is the
// GoReleaser that publishes them (issue #147).
const ciGoreleaserInstall = `      # The same goreleaser version release.yml runs, installed only: the script below runs a snapshot of
      # the release at this commit, which builds the archives and publishes nothing.
      - uses: goreleaser/goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de # v7.3.0
        with:
          distribution: goreleaser
          version: "v2.20.1" # exact pin, as release.yml's; Renovate updates it (github-actions manager)
          install-only: true
`

// ciArchiveStep is the step of .github/workflows/ci.yml's test job that runs
// the archive check's script.
const ciArchiveStep = `      # The release's archives, built by a snapshot run of its GoReleaser
      # configuration: each holds exactly its own binary (issue #147).
      - name: Each archive holds its own binary
        run: .github/scripts/verify-archives.sh
`

// ciArchiveCheck is the part of .github/workflows/ci.yml that issue #147 adds:
// the dry run of the release's archives. The test job, which runs on every pull
// request and every push to main, installs the release's GoReleaser and runs
// .github/scripts/verify-archives.sh, which builds the release's archives and
// fails unless each holds exactly its own binary: the CLI's archive shift2eu
// and not shift2eu-server, the server's shift2eu-server and not shift2eu.
const ciArchiveCheck = `name: CI

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: go.mod
` + ciGoreleaserInstall + ciArchiveStep

// TestDryRunVerifiesArchives pins the other half of issue #147: the CI dry run
// verifies what the release publishes, as ci.yml's image job does for the
// images. check.DryRunVerifiesArchives reads ci.yml and release.yml and returns
// one message unless the test job, jobs: test, holds exactly one step named
//
//	Each archive holds its own binary
//
// whose run: is exactly the one line
//
//	.github/scripts/verify-archives.sh
//
// and nothing else: no literal block, no argument and no ${{ }} expression, so
// that the script, which builds the release's archives with a goreleaser
// snapshot and fails unless each archive holds exactly its own binary, is what
// runs on every pull request and every push to main. A message names the job
// (jobs: test) or the script path it wants, and the step name.
//
// The dry run also installs the GoReleaser the release runs, so that a Renovate
// pull request bumping one workflow cannot let CI check the archives with a
// different GoReleaser than the one that publishes them (issue #147): the test
// job holds exactly one goreleaser/goreleaser-action step, before the archive
// check's step, whose uses: (the full commit), with: distribution: and with:
// version: are release.yml's goreleaser step's, and whose with: install-only:
// is true (the release publishes, the dry run only builds). A message names the
// field that differs (uses, distribution, version, install-only), or the step
// that is missing, repeated or misplaced; a step after the archive check is
// worded with before.
func TestDryRunVerifiesArchives(t *testing.T) {
	t.Parallel()

	release := releaseWorkflowFixture()
	releaseWorkflow := release.render()

	tests := []struct {
		name string
		ci   string
		// release is release.yml the case compares ci against; "" means
		// releaseWorkflow, unchanged.
		release string
		// wantIn is a part of the message the check must return; "" means
		// none at all.
		wantIn string
	}{
		// The accepted shape.
		{name: "the test job runs the archive check", ci: ciArchiveCheck},
		{
			name: "the accepted workflow with CRLF line ends",
			ci:   strings.ReplaceAll(ciArchiveCheck, "\n", "\r\n"),
		},

		// The step must exist, under its own name.
		{
			name:   "no step verifying the archives",
			ci:     mutate(t, ciArchiveCheck, "      - name: Each archive holds its own binary\n        run: .github/scripts/verify-archives.sh\n", ""),
			wantIn: "Each archive holds its own binary",
		},
		{
			name:   "the step renamed",
			ci:     mutate(t, ciArchiveCheck, "      - name: Each archive holds its own binary\n", "      - name: Check the archives\n"),
			wantIn: "Each archive holds its own binary",
		},

		// The step must run the archive check's script, and run it whole.
		{
			name:   "the step running another script",
			ci:     mutate(t, ciArchiveCheck, "        run: .github/scripts/verify-archives.sh\n", "        run: .github/scripts/other.sh\n"),
			wantIn: ".github/scripts/verify-archives.sh",
		},
		{
			name:   "the step running the script with an argument",
			ci:     mutate(t, ciArchiveCheck, "        run: .github/scripts/verify-archives.sh\n", "        run: .github/scripts/verify-archives.sh --all\n"),
			wantIn: ".github/scripts/verify-archives.sh",
		},
		{
			// A literal block is compared line by line, so a script the
			// workflow writes itself would be the check, not the script's.
			name:   "the step running a script of its own",
			ci:     mutate(t, ciArchiveCheck, "        run: .github/scripts/verify-archives.sh\n", "        run: |\n          echo ok\n"),
			wantIn: ".github/scripts/verify-archives.sh",
		},

		// The step must sit in the test job, which runs on every pull request
		// and every push to main.
		{
			name:   "the step in another job",
			ci:     mutate(t, ciArchiveCheck, "  test:\n", "  lint:\n"),
			wantIn: "jobs: test",
		},

		// The dry run installs the GoReleaser the release runs: the same
		// action commit, distribution and version, installed only.
		{
			name:   "no goreleaser-action step in the test job",
			ci:     mutate(t, ciArchiveCheck, ciGoreleaserInstall, ""),
			wantIn: "goreleaser/goreleaser-action",
		},
		{
			// The snapshot would not find the action installed at all.
			name:   "the goreleaser-action step after the archive check",
			ci:     mutate(t, ciArchiveCheck, ciGoreleaserInstall+ciArchiveStep, ciArchiveStep+ciGoreleaserInstall),
			wantIn: "before",
		},
		{
			// A Renovate pull request bumping ci.yml alone.
			name:   "a goreleaser-action commit that is not the release's",
			ci:     mutate(t, ciArchiveCheck, "@a4f02ab5355a589de60d917a659ce617ed19b3de", "@1111111111111111111111111111111111111111"),
			wantIn: "uses",
		},
		{
			name:   "a goreleaser version that is not the release's",
			ci:     mutate(t, ciArchiveCheck, `version: "v2.20.1"`, `version: "v2.20.2"`),
			wantIn: "version",
		},
		{
			name:   "a goreleaser distribution that is not the release's",
			ci:     mutate(t, ciArchiveCheck, "distribution: goreleaser", "distribution: goreleaser-pro"),
			wantIn: "distribution",
		},
		{
			// Renovate bumping release.yml's version instead: the drift is
			// the same, whichever workflow moved.
			name:    "a release version that is not the dry run's",
			ci:      ciArchiveCheck,
			release: release.inJob(t, "goreleaser", `version: "v2.20.1"`, `version: "v2.21.0"`),
			wantIn:  "version",
		},
		{
			// Without install-only the step runs the action's own
			// `goreleaser release`, publishing a snapshot to nowhere.
			name:   "a goreleaser-action step that is not install-only",
			ci:     mutate(t, ciArchiveCheck, "          install-only: true\n", ""),
			wantIn: "install-only",
		},
		{
			name:   "an install-only that is false",
			ci:     mutate(t, ciArchiveCheck, "          install-only: true\n", "          install-only: false\n"),
			wantIn: "install-only",
		},
		{
			// Two installs of GoReleaser: one of them may be the wrong one.
			name:   "two goreleaser-action steps",
			ci:     mutate(t, ciArchiveCheck, ciGoreleaserInstall, ciGoreleaserInstall+ciGoreleaserInstall),
			wantIn: "goreleaser/goreleaser-action",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			releaseDoc := tc.release
			if releaseDoc == "" {
				releaseDoc = releaseWorkflow
			}

			got := check.DryRunVerifiesArchives([]byte(tc.ci), []byte(releaseDoc))
			joined := strings.Join(got, "\n")

			if tc.wantIn == "" {
				if len(got) != 0 {
					t.Errorf("DryRunVerifiesArchives() = %q, want no message", got)
				}

				return
			}

			if !strings.Contains(joined, tc.wantIn) {
				t.Errorf("DryRunVerifiesArchives() = %q, want a message containing %q", got, tc.wantIn)
			}
		})
	}
}
