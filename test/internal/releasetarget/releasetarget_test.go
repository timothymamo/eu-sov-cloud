package releasetarget_test

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/test/internal/releasetarget"
)

// The .goreleaser.yaml documents below are hand-written copies of this
// repository's build, cut down to the keys Read reads. The build has
// one build, CGO_ENABLED=0, and three goos by two goarch: six platforms, which
// is the list every case below compares against or breaks.
//
// buildFlow and buildReal live in testdata, as files: the tests of
// test/architecture/internal/check add a release section to the same two
// documents, so there is one copy of each to keep in step with the release.
var (
	// buildFlow writes the goos and goarch lists in flow form, as
	// .goreleaser.yaml does.
	//
	//go:embed testdata/flow.goreleaser.yaml
	buildFlow string

	// buildReal is a snapshot of the shape of this repository's
	// .goreleaser.yaml: before hooks, archives, checksum and changelog around
	// one build. It is not a copy of that file. The check may refuse a
	// top-level key that could change what the release builds, so a document
	// of this shape must stay accepted; test/architecture/repository_test.go
	// checks the real .goreleaser.yaml itself.
	//
	//go:embed testdata/real.goreleaser.yaml
	buildReal string
)

const (
	// buildBlock writes the same lists in block form.
	buildBlock = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    binary: eusov
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
`

	// buildEnvFlow keeps the flow lists and writes env in flow form too, so a
	// reader that only understands one form of either list fails here.
	buildEnvFlow = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
`

	// buildCgoOn turns cgo on: a release build that would link the host's C
	// library and so depend on what the host has installed.
	buildCgoOn = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=1
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
`

	// buildNoEnv leaves the environment out, so cgo falls back to the default
	// of the toolchain that reads it.
	buildNoEnv = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
`

	// buildEmptyGoos asks for no platform at all.
	buildEmptyGoos = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: []
    goarch: [amd64, arm64]
`

	// buildNoGoarch leaves arch selection to whatever the toolchain defaults
	// to, which is not the list the release was reviewed for.
	buildNoGoarch = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
`

	// buildTwo makes two binaries, each with its own target list, so the
	// platforms of one document are no longer one list.
	buildTwo = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
  - id: eusov-helper
    main: ./cmd/eusov-helper
    env:
      - CGO_ENABLED=0
    goos: [linux]
    goarch: [amd64]
`

	// buildIgnore drops platforms from the list with a matrix of its own, so
	// the goos x goarch product is not what the release builds.
	buildIgnore = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ignore:
      - goos: darwin
        goarch: arm64
`

	// buildTargets names platforms one by one instead of crossing goos with
	// goarch, in the form goreleaser v1 took.
	buildTargets = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    targets:
      - linux_amd64
      - darwin_arm64
`

	// buildTopGomod sets the Go binary goreleaser builds with, so the release
	// would compile with a toolchain the check never read. It is written in
	// block form, as the file would hold it.
	buildTopGomod = `version: 2

gomod:
  gobinary: go1.22.0

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
`

	// buildBeforeGenerate runs a second hook before the build, which could
	// generate, rewrite or delete the files the release compiles.
	buildBeforeGenerate = `version: 2

before:
  hooks:
    - go mod tidy
    - go generate ./...

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
`

	// buildBeforeOtherKey puts something other than hooks under before, which
	// the check does not read.
	buildBeforeOtherKey = `version: 2

before:
  env:
    - FOO=bar

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
`

	// buildRepeatedGoos lists the goos key twice: the reader must refuse it
	// rather than keep one of the two lists.
	buildRepeatedGoos = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: [linux]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
`

	// buildTemplatedGoos asks for a platform goreleaser computes from a
	// template, so the list the check reads is not the list the release
	// builds.
	buildTemplatedGoos = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, '{{ .GOOS }}']
    goarch: [amd64, arm64]
`

	// buildMergeKey merges another mapping into the document, whose keys the
	// check would never see.
	buildMergeKey = `version: 2

<<: *base

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
`

	// buildAnchoredEnv anchors the env list, so the list the check reads is
	// not necessarily the one goreleaser expands.
	buildAnchoredEnv = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env: &envs
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
`

	// buildAliasedGoos takes its platform list from an alias: the reader does
	// not resolve aliases, so it cannot read the list.
	buildAliasedGoos = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: *gos
    goarch: [amd64, arm64]
`

	// buildCRInComment is the auditor's probe: a CR inside the comment on the
	// goarch key. A reader that takes CR for a line end sees the `- riscv64`
	// entry outside the comment and lists nine platforms instead of failing.
	buildCRInComment = `version: 2

builds:
  - id: eusov
    main: ./cmd/eusov
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: # amd64 and arm64 only` + "\r" + `      - riscv64
      - amd64
      - arm64
`
)

// buildDoc returns a .goreleaser.yaml with one build: the lines given, each
// already indented under the build's dash, then the six-platform goos and
// goarch lists.
func buildDoc(lines string) string {
	return "version: 2\n\nbuilds:\n  - id: eusov\n    main: ./cmd/eusov\n" + lines +
		"    goos: [linux, darwin, windows]\n    goarch: [amd64, arm64]\n"
}

// buildLists returns a .goreleaser.yaml with one build whose goos and goarch
// lists are exactly the two lines given, each already indented under the
// build's dash: for a case about a list itself rather than about the keys
// around it.
func buildLists(goos, goarch string) string {
	return "version: 2\n\nbuilds:\n  - id: eusov\n    main: ./cmd/eusov\n    env:\n      - CGO_ENABLED=0\n" +
		goos + "\n" + goarch + "\n"
}

// buildWithComment returns buildReal with comment added as a comment line
// above project_name. The reader skips comments, so a character the check must
// refuse is the only reason a case using it fails.
func buildWithComment(comment string) string {
	return strings.Replace(buildReal, "project_name: eusov", comment+"\nproject_name: eusov", 1)
}

// TestRead pins the platform list the release builds: every goos x
// goarch pair of this repository's one build, sorted by GOOS then GOARCH, and
// an error for every document the check cannot read the whole list from.
func TestRead(t *testing.T) {
	t.Parallel()

	// allSix is the goos x goarch product of buildFlow and buildBlock, sorted
	// by GOOS then GOARCH.
	allSix := []releasetarget.Target{
		{GOOS: "darwin", GOARCH: "amd64"},
		{GOOS: "darwin", GOARCH: "arm64"},
		{GOOS: "linux", GOARCH: "amd64"},
		{GOOS: "linux", GOARCH: "arm64"},
		{GOOS: "windows", GOARCH: "amd64"},
		{GOOS: "windows", GOARCH: "arm64"},
	}

	tests := []struct {
		name       string
		goreleaser string
		want       []releasetarget.Target
		wantErr    bool
	}{
		{
			name:       "flow lists give every pair",
			goreleaser: buildFlow,
			want:       allSix,
		},
		{
			name:       "block lists give every pair",
			goreleaser: buildBlock,
			want:       allSix,
		},
		{
			name:       "an env list in flow form still turns cgo off",
			goreleaser: buildEnvFlow,
			want:       allSix,
		},
		{
			name:       "the real document, with its before hooks and trailing keys, gives every pair",
			goreleaser: buildReal,
			want:       allSix,
		},
		{
			name:       "CRLF line ends throughout still give every pair",
			goreleaser: strings.ReplaceAll(buildReal, "\n", "\r\n"),
			want:       allSix,
		},
		{
			name:       "cgo enabled is refused",
			goreleaser: buildCgoOn,
			wantErr:    true,
		},
		{
			name:       "no env is refused",
			goreleaser: buildNoEnv,
			wantErr:    true,
		},
		{
			name:       "an empty goos list is refused",
			goreleaser: buildEmptyGoos,
			wantErr:    true,
		},
		{
			name:       "a missing goarch key is refused",
			goreleaser: buildNoGoarch,
			wantErr:    true,
		},
		{
			name:       "two builds are refused",
			goreleaser: buildTwo,
			wantErr:    true,
		},
		{
			name:       "an ignore list is refused",
			goreleaser: buildIgnore,
			wantErr:    true,
		},
		{
			name:       "a targets list is refused",
			goreleaser: buildTargets,
			wantErr:    true,
		},
		{
			name:       "an empty document is refused",
			goreleaser: "",
			wantErr:    true,
		},

		// A top-level key the checker does not read could change what the
		// release builds, so only the keys the real document holds pass.
		{
			name:       "a top-level env is refused",
			goreleaser: buildFlow + "\nenv:\n  - FOO=bar\n",
			wantErr:    true,
		},
		{
			name:       "a top-level gomod is refused",
			goreleaser: buildTopGomod,
			wantErr:    true,
		},
		{
			name:       "an unknown top-level key is refused",
			goreleaser: buildFlow + "\nupx:\n  - enabled: true\n",
			wantErr:    true,
		},
		{
			// release says how the GitHub release is published, not what is
			// compiled: GitHubRelease holds it to its one setting.
			name:       "a release section still gives every pair",
			goreleaser: buildFlow + "\nrelease:\n  prerelease: auto\n",
			want:       allSix,
		},
		{
			name:       "a document marker is refused",
			goreleaser: "---\n" + buildFlow,
			wantErr:    true,
		},
		{
			name:       "a merge key is refused",
			goreleaser: buildMergeKey,
			wantErr:    true,
		},

		// before: hooks run before the build, so a hook the check does not
		// know could generate or rewrite what the release compiles.
		{
			name:       "an added before hook is refused",
			goreleaser: buildBeforeGenerate,
			wantErr:    true,
		},
		{
			name:       "a key under before other than hooks is refused",
			goreleaser: buildBeforeOtherKey,
			wantErr:    true,
		},

		// The build environment the release was reviewed for is exactly one
		// entry, CGO_ENABLED=0.
		{
			name:       "a second env entry beside CGO_ENABLED=0 is refused",
			goreleaser: buildDoc("    env:\n      - CGO_ENABLED=0\n      - FOO=bar\n"),
			wantErr:    true,
		},
		{
			name:       "a GOFLAGS entry beside CGO_ENABLED=0 is refused",
			goreleaser: buildDoc("    env:\n      - CGO_ENABLED=0\n      - GOFLAGS=-tags=x\n"),
			wantErr:    true,
		},
		{
			name:       "a templated env name is refused",
			goreleaser: buildDoc("    env:\n      - CGO_ENABLED=0\n      - '{{ \"GOFLAGS\" }}=-tags=sneaky'\n"),
			wantErr:    true,
		},
		{
			name:       "CGO_ENABLED=0 listed twice is refused",
			goreleaser: buildDoc("    env:\n      - CGO_ENABLED=0\n      - CGO_ENABLED=0\n"),
			wantErr:    true,
		},

		// The build keys the check does not read are exactly the ones that
		// cannot change which files the release compiles.
		{
			name:       "a tags key is refused",
			goreleaser: buildDoc("    env:\n      - CGO_ENABLED=0\n    tags:\n      - sneaky\n"),
			wantErr:    true,
		},
		{
			name:       "an overrides key is refused",
			goreleaser: buildDoc("    env:\n      - CGO_ENABLED=0\n    overrides:\n      - goos: linux\n        goarch: arm64\n"),
			wantErr:    true,
		},
		{
			name:       "a goamd64 key is refused",
			goreleaser: buildDoc("    env:\n      - CGO_ENABLED=0\n    goamd64:\n      - v3\n"),
			wantErr:    true,
		},
		{
			name:       "a flag other than -trimpath is refused",
			goreleaser: buildDoc("    env:\n      - CGO_ENABLED=0\n    flags:\n      - -trimpath\n      - -tags=x\n"),
			wantErr:    true,
		},
		{
			name:       "a templated goos entry is refused",
			goreleaser: buildTemplatedGoos,
			wantErr:    true,
		},
		{
			name:       "a repeated key is refused",
			goreleaser: buildRepeatedGoos,
			wantErr:    true,
		},

		// A ' #' starts a comment, and a comment runs to the end of the line:
		// a closing bracket inside one does not close the list, so the document
		// never says which platforms the release builds.
		{
			name:       "a goos flow list whose closing bracket sits in a comment is refused",
			goreleaser: buildLists("    goos: [linux # ]", "    goarch: [amd64, arm64]"),
			wantErr:    true,
		},
		{
			name:       "a goarch flow list whose closing bracket sits in a comment is refused",
			goreleaser: buildLists("    goos: [linux, darwin, windows]", "    goarch: [amd64 # ]"),
			wantErr:    true,
		},
		{
			name:       "an anchored env list is refused",
			goreleaser: buildAnchoredEnv,
			wantErr:    true,
		},
		{
			name:       "an aliased goos list is refused",
			goreleaser: buildAliasedGoos,
			wantErr:    true,
		},

		// The characters the document may hold: a comment is no place for a
		// control character either, since no two YAML readers agree on what
		// the ones that look like line breaks do.
		{
			name:       "a lone CR is refused",
			goreleaser: buildWithComment("# a lone\rCR"),
			wantErr:    true,
		},
		{
			name:       "U+0085 is refused",
			goreleaser: buildWithComment("# a NEL:\u0085x"),
			wantErr:    true,
		},
		{
			name:       "U+2028 is refused",
			goreleaser: buildWithComment("# a line separator:\u2028x"),
			wantErr:    true,
		},
		{
			name:       "U+2029 is refused",
			goreleaser: buildWithComment("# a paragraph separator:\u2029x"),
			wantErr:    true,
		},
		{
			name:       "a byte-order mark is refused",
			goreleaser: buildWithComment("#\uFEFF a byte-order mark"),
			wantErr:    true,
		},
		{
			name:       "a tab is refused",
			goreleaser: buildWithComment("#\ta tab"),
			wantErr:    true,
		},
		{
			name:       "another C0 control is refused",
			goreleaser: buildWithComment("#\x01a control"),
			wantErr:    true,
		},
		{
			name:       "DEL is refused",
			goreleaser: buildWithComment("#\x7fDEL"),
			wantErr:    true,
		},
		{
			name:       "invalid UTF-8 is refused",
			goreleaser: buildWithComment("#\xff not UTF-8"),
			wantErr:    true,
		},
		{
			name:       "the auditor's CR inside the goarch comment is an error, not six platforms",
			goreleaser: buildCRInComment,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := releasetarget.Read([]byte(tc.goreleaser))

			if tc.wantErr {
				if err == nil {
					t.Fatalf("Read() = %v, nil; want an error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("Read(): %v", err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Read() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
