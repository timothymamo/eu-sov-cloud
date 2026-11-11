package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// releaseWorkflow is the shape of .github/workflows/release.yml the check
// accepts: a test job, and a goreleaser job that needs it, each with exactly
// the steps the check lists.
const releaseWorkflow = `name: Release

on:
  push:
    tags: ["v*.*.*"]

permissions: {}

jobs:
  test:
    name: Test
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: go.mod
          cache: false
      - name: Test
        run: make test

  goreleaser:
    name: GoReleaser
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: write # create the GitHub release
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: go.mod
          cache: false
      - uses: goreleaser/goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de # v7.3.0
        with:
          distribution: goreleaser
          version: "v2.20.1" # exact pin
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
`

// TestReleaseWorkflow pins what the release workflow may say. It is read as a
// closed schema: the top-level keys, the jobs, their keys and their steps are
// fixed, so no step runs repository code, sets the go command's environment or
// swaps the goreleaser document before goreleaser runs. The probes are the
// steps the second review and audit of issue #40 found the lexical check let
// through.
func TestReleaseWorkflow(t *testing.T) {
	t.Parallel()

	const gorelStep = "      - uses: goreleaser/goreleaser-action@"

	// beforeGoreleaser inserts step before the goreleaser step.
	beforeGoreleaser := func(step string) string {
		return strings.Replace(releaseWorkflow, gorelStep, "      - "+step+"\n"+gorelStep, 1)
	}

	// inTestJob inserts step before the test job's make test.
	inTestJob := func(step string) string {
		return strings.Replace(releaseWorkflow, "      - name: Test\n", "      - "+step+"\n      - name: Test\n", 1)
	}

	tests := []struct {
		name     string
		workflow string
		// wantIn is a part of the message the check must return; "" means
		// none at all.
		wantIn string
	}{
		{name: "the accepted shape", workflow: releaseWorkflow},
		{name: "CRLF line ends", workflow: strings.ReplaceAll(releaseWorkflow, "\n", "\r\n")},
		{
			name:     "a newer pin of every action and of goreleaser",
			workflow: strings.ReplaceAll(strings.ReplaceAll(releaseWorkflow, "a4f02ab5355a589de60d917a659ce617ed19b3de", "0123456789abcdef0123456789abcdef01234567"), `"v2.20.1"`, `"v2.21.0"`),
		},

		// The probes of the review and audit, each before the goreleaser step.
		{name: "probe: go env -w GOFLAGS", workflow: beforeGoreleaser(`run: go env -w "G""OFLAGS=-tags=sneaky"`), wantIn: "holds 4 steps"},
		{name: "probe: GOFLAGS to github.env", workflow: beforeGoreleaser(`run: echo "G""OFLAGS=-tags=x" >> "${{ github.env }}"`), wantIn: "github.env"},
		{name: "probe: GITHUB_ENV spelled at run time", workflow: beforeGoreleaser(`run: n=GITHUB_E; n=${n}NV; echo X=1 >> "${!n}"`), wantIn: "holds 4 steps"},
		{name: "probe: go work init", workflow: beforeGoreleaser(`run: go work init .`), wantIn: "holds 4 steps"},
		{name: "probe: go mod vendor", workflow: beforeGoreleaser(`run: go mod vendor`), wantIn: "holds 4 steps"},
		{name: "probe: another goreleaser document copied in", workflow: beforeGoreleaser(`run: cp build/other.yaml .goreleaser.yaml`), wantIn: "holds 4 steps"},
		{name: "probe: an action that sets the environment", workflow: beforeGoreleaser(`uses: example/sets-env@0123456789abcdef0123456789abcdef01234567`), wantIn: "holds 4 steps"},
		{name: "probe: a repository script", workflow: beforeGoreleaser(`run: ./scripts/prepare.sh`), wantIn: "holds 4 steps"},
		{name: "probe: make test back in the goreleaser job", workflow: beforeGoreleaser(`run: make test`), wantIn: "holds 4 steps"},
		{name: "a step added to the test job", workflow: inTestJob(`run: go env -w GOFLAGS=-x`), wantIn: "holds 4 steps"},
		{
			name:     "a step replaced in the goreleaser job",
			workflow: strings.Replace(releaseWorkflow, "      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0\n        with:\n          go-version-file: go.mod\n          cache: false\n      - uses: goreleaser", "      - run: ./scripts/prepare.sh\n      - uses: goreleaser", 1),
			wantIn:   "step 2",
		},
		{
			name:     "the test job running another command",
			workflow: strings.Replace(releaseWorkflow, "run: make test", "run: make test && go env -w GOFLAGS=-x", 1),
			wantIn:   "run",
		},

		// setup-go's Actions cache (GOMODCACHE and GOCACHE) would carry
		// state from the test job, or from CI on main, into the goreleaser
		// job, so both jobs turn it off.
		{name: "setup-go caching in the test job", workflow: strings.Replace(releaseWorkflow, "          cache: false\n      - name: Test", "      - name: Test", 1), wantIn: `jobs: test: steps: step 2`},
		{name: "setup-go caching in the goreleaser job", workflow: strings.Replace(releaseWorkflow, "          cache: false\n      - uses: goreleaser", "      - uses: goreleaser", 1), wantIn: `jobs: goreleaser: steps: step 2`},
		{name: "setup-go cache: true", workflow: strings.Replace(releaseWorkflow, "          cache: false\n      - uses: goreleaser", "          cache: true\n      - uses: goreleaser", 1), wantIn: `with: cache is "true"`},

		// The tokens through which a step sets the environment or the PATH of
		// the steps after it, wherever they are, comments included.
		{name: "github.path in a comment", workflow: strings.Replace(releaseWorkflow, "# v7.3.0", "# ${{ github.path }}", 1), wantIn: "github.path"},
		{name: "GITHUB.ENV in upper case", workflow: strings.Replace(releaseWorkflow, "# v7.3.0", "# ${{ GITHUB.ENV }}", 1), wantIn: "github.env"},
		{name: "GITHUB_ENV in a comment", workflow: strings.Replace(releaseWorkflow, "# v7.3.0", "# GITHUB_ENV", 1), wantIn: "GITHUB_ENV"},
		{name: "GOEXPERIMENT in a comment", workflow: strings.Replace(releaseWorkflow, "# v7.0.1", "# GOEXPERIMENT", 1), wantIn: "GOEXPERIMENT"},

		// The keys around the steps.
		{name: "a workflow-level env", workflow: strings.Replace(releaseWorkflow, "jobs:\n", "env:\n  SNEAKY: x\njobs:\n", 1), wantIn: `key "env"`},
		{name: "a workflow-level defaults", workflow: strings.Replace(releaseWorkflow, "jobs:\n", "defaults:\n  run:\n    shell: bash\njobs:\n", 1), wantIn: `key "defaults"`},
		{name: "a third job", workflow: releaseWorkflow + "  other:\n    runs-on: ubuntu-latest\n", wantIn: `key "other"`},
		{name: "a job-level env", workflow: strings.Replace(releaseWorkflow, "    needs: test\n", "    needs: test\n    env:\n      GOFLAGS: -tags=sneaky\n", 1), wantIn: `key "env"`},
		{name: "a container", workflow: strings.Replace(releaseWorkflow, "    needs: test\n", "    needs: test\n    container: golang\n", 1), wantIn: `key "container"`},
		{name: "no needs: test", workflow: strings.Replace(releaseWorkflow, "    needs: test\n", "", 1), wantIn: `no "needs"`},
		{name: "needs another job", workflow: strings.Replace(releaseWorkflow, "    needs: test\n", "    needs: [test]\n", 1), wantIn: "needs"},
		{name: "write permission at the top", workflow: strings.Replace(releaseWorkflow, "permissions: {}", "permissions:\n  contents: write", 1), wantIn: "permissions"},
		{name: "write permission for the tests", workflow: strings.Replace(releaseWorkflow, "contents: read", "contents: write", 1), wantIn: "contents"},
		{name: "another trigger", workflow: strings.Replace(releaseWorkflow, "  push:\n", "  workflow_dispatch:\n  push:\n", 1), wantIn: "workflow_dispatch"},
		{name: "a step-level if", workflow: strings.Replace(releaseWorkflow, "        run: make test\n", "        run: make test\n        if: false\n", 1), wantIn: `key "if"`},
		{name: "a working-directory", workflow: strings.Replace(releaseWorkflow, "        run: make test\n", "        run: make test\n        working-directory: sub\n", 1), wantIn: `key "working-directory"`},
		{name: "an env on another step", workflow: strings.Replace(releaseWorkflow, "          cache: false\n      - uses: goreleaser", "          cache: false\n        env:\n          A: b\n      - uses: goreleaser", 1), wantIn: `key "env"`},

		// What goreleaser is asked to do.
		{name: "an argument glued to --clean", workflow: strings.Replace(releaseWorkflow, "args: release --clean", "args: release --clean-f.goreleaser.prod.yaml", 1), wantIn: "args"},
		{name: "a --config", workflow: strings.Replace(releaseWorkflow, "args: release --clean", "args: release --clean --config x.yaml", 1), wantIn: "args"},
		{name: "no args", workflow: strings.Replace(releaseWorkflow, "          args: release --clean\n", "", 1), wantIn: `no "args"`},
		{name: "a workdir", workflow: strings.Replace(releaseWorkflow, "distribution: goreleaser", "distribution: goreleaser\n          workdir: sub", 1), wantIn: `key "workdir"`},
		{name: "a goreleaser version range", workflow: strings.Replace(releaseWorkflow, `"v2.20.1"`, `"~> v2"`, 1), wantIn: "version"},
		{name: "an action pinned by tag", workflow: strings.Replace(releaseWorkflow, "goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de", "goreleaser-action@v7", 1), wantIn: "goreleaser-action@v7"},
		{name: "a second key in the goreleaser env", workflow: strings.Replace(releaseWorkflow, "GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n", "GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n          OTHER: x\n", 1), wantIn: "OTHER"},
		{name: "a GITHUB_TOKEN from somewhere else", workflow: strings.Replace(releaseWorkflow, "${{ secrets.GITHUB_TOKEN }}", "${{ vars.TOKEN }}", 1), wantIn: "GITHUB_TOKEN"},
		{name: "a flow-style env", workflow: strings.Replace(releaseWorkflow, "        env:\n          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n", "        env: {GITHUB_TOKEN: x, A: b}\n", 1), wantIn: "env"},
		{name: "a repeated key", workflow: strings.Replace(releaseWorkflow, "    needs: test\n", "    needs: test\n    needs: test\n", 1), wantIn: "repeated"},
		{name: "a tab", workflow: strings.Replace(releaseWorkflow, "# v7.0.1", "#\tv7.0.1", 1), wantIn: "U+0009"},
		{name: "an empty document", workflow: "", wantIn: "release workflow"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := check.ReleaseWorkflow([]byte(tc.workflow))

			if tc.wantIn == "" {
				if len(got) != 0 {
					t.Errorf("ReleaseWorkflow() = %q, want no message", got)
				}

				return
			}

			if !strings.Contains(strings.Join(got, "\n"), tc.wantIn) {
				t.Errorf("ReleaseWorkflow() = %q, want a message containing %q", got, tc.wantIn)
			}
		})
	}
}

// TestStrayReleaseFiles pins the files at the repository root that change what
// goreleaser builds without the check seeing them: the other goreleaser
// document names, and go.work, go.work.sum and vendor, which the go command
// goreleaser runs reads and the check's go list (GOWORK=off, -mod=readonly)
// does not. What counts is what git could commit, as git ls-files --cached
// --others --exclude-standard lists it: a developer's ignored go.work never
// reaches the release, a tracked or force-added one does.
func TestStrayReleaseFiles(t *testing.T) {
	t.Parallel()

	names := []string{
		".config/goreleaser.yml",
		".config/goreleaser.yaml",
		".goreleaser.yml",
		"goreleaser.yml",
		"goreleaser.yaml",
		"go.work",
		"go.work.sum",
	}

	// repo returns a new git repository that ignores go.work and go.work.sum,
	// as the repository's .gitignore does.
	repo := func(t *testing.T) string {
		t.Helper()

		root := t.TempDir()
		git(t, root, "init", "--quiet")
		writeFileContent(t, root, ".gitignore", "go.work\ngo.work.sum\n")

		return root
	}

	t.Run("a clean root", func(t *testing.T) {
		t.Parallel()

		root := repo(t)
		writeFile(t, root, ".goreleaser.yaml")
		writeFile(t, root, "go.mod")
		writeFile(t, root, "sub/vendor/x.go")
		writeFile(t, root, "sub/go.work")
		writeFile(t, root, "vendored/x.go")

		assertNoStray(t, root)
	})

	t.Run("an ignored go.work and go.work.sum", func(t *testing.T) {
		t.Parallel()

		root := repo(t)
		writeFile(t, root, "go.work")
		writeFile(t, root, "go.work.sum")

		assertNoStray(t, root)
	})

	for _, name := range names {
		t.Run("untracked "+name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			git(t, root, "init", "--quiet")
			writeFile(t, root, name)

			assertStray(t, root, name)
		})

		t.Run("force-added "+name, func(t *testing.T) {
			t.Parallel()

			root := repo(t)
			writeFile(t, root, name)
			git(t, root, "add", "--force", name)

			assertStray(t, root, name)
		})
	}

	t.Run("a tracked go.work gone from the working tree", func(t *testing.T) {
		t.Parallel()

		root := repo(t)
		writeFile(t, root, "go.work")
		git(t, root, "add", "--force", "go.work")

		if err := os.Remove(filepath.Join(root, "go.work")); err != nil {
			t.Fatalf("remove go.work: %v", err)
		}

		assertStray(t, root, "go.work")
	})

	t.Run("an untracked, not ignored vendor directory", func(t *testing.T) {
		t.Parallel()

		root := repo(t)
		writeFile(t, root, "vendor/modules.txt")
		writeFile(t, root, "vendor/example.com/m/x.go")

		assertStray(t, root, "vendor")
	})

	t.Run("a go.work symlink to nothing", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		git(t, root, "init", "--quiet")

		if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "go.work")); err != nil {
			t.Fatalf("create the symlink: %v", err)
		}

		assertStray(t, root, "go.work")
	})

	t.Run("outside git", func(t *testing.T) {
		t.Parallel()

		// A .git file pointing nowhere stops git from looking further up for
		// a repository, so git fails here.
		root := t.TempDir()
		writeFileContent(t, root, ".git", "gitdir: missing\n")

		if got, err := check.StrayReleaseFiles(t.Context(), root); err == nil {
			t.Fatalf("StrayReleaseFiles() = %q, nil; want an error", got)
		}
	})
}

func assertNoStray(t *testing.T, root string) {
	t.Helper()

	got, err := check.StrayReleaseFiles(t.Context(), root)
	if err != nil {
		t.Fatalf("StrayReleaseFiles(): %v", err)
	}

	if len(got) != 0 {
		t.Errorf("StrayReleaseFiles() = %q, want no message", got)
	}
}

func assertStray(t *testing.T, root, name string) {
	t.Helper()

	got, err := check.StrayReleaseFiles(t.Context(), root)
	if err != nil {
		t.Fatalf("StrayReleaseFiles(): %v", err)
	}

	if len(got) != 1 || !strings.HasPrefix(got[0], name+" ") {
		t.Errorf("StrayReleaseFiles() = %q, want one message starting %q", got, name+" ")
	}
}

// writeFile creates the file name, slash-separated, under root, with the
// directories it needs.
func writeFile(t *testing.T, root, name string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create the directory for %s: %v", name, err)
	}

	if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// writeFileContent creates the file name, slash-separated, under root, with
// content and the directories it needs.
func writeFileContent(t *testing.T, root, name, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create the directory for %s: %v", name, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
