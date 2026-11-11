package check_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// releaseWorkflowWithoutImage is .github/workflows/release.yml up to its image
// job: the test job, and the goreleaser job that needs it, as the file stands,
// comments included, so the check must tolerate a full-line comment above a
// step and a trailing comment on a value.
const releaseWorkflowWithoutImage = `name: Release

on:
  push:
    tags: ["v*.*.*"]

# No permission by default; each job asks for its own.
permissions: {}

# The steps of all three jobs are a closed list that test/architecture holds
# (check.ReleaseWorkflow): add no step, run: or env: here without changing it.
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
          cache: false # see the comment on the goreleaser job
      - name: Test
        run: make test

  # A job of its own, on a fresh runner: no go env file and no environment or
  # PATH that the tests set reaches goreleaser. setup-go's Actions cache
  # (module and build cache) would carry state over from the test job or from
  # CI on main, so both jobs set cache: false.
  goreleaser:
    name: GoReleaser
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: write # create the GitHub release and upload artifacts
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
          version: "v2.20.1" # exact pin; Renovate updates it (github-actions manager)
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
`

// imageJobHead is the image job of .github/workflows/release.yml up to its
// steps:, with the full-line comments the check must let through between the
// job's keys.
const imageJobHead = `
  # Publishes the container image to GHCR, signed, with provenance and an SBOM
  # (ADR 0014). A job of its own, so the write permissions it needs never meet
  # the goreleaser job's contents: write. It builds from the Dockerfile with
  # the build arguments GoReleaser stamps into the release binaries, and
  # carries no build cache in: BuildKit starts empty on a fresh runner.
  image:
    name: Container image
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: read # check out the repository
      packages: write # push the image, its signature and attestations to ghcr.io
      id-token: write # GitHub OIDC token for keyless cosign and attestation signing
      attestations: write # store the provenance and SBOM attestations in the repository
    steps:
`

// imageJobSteps are the image job's steps, in the order release.yml writes
// them, each with the comment lines above it: the check must tolerate a
// full-line comment between the steps, in a with: mapping and on a value.
var imageJobSteps = []string{
	`      # The whole history, as GoReleaser checks it out: git picks the length of
      # a short commit from the number of objects it holds.
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
`,
	`      # The values GoReleaser stamps into the release binaries: .Version (the
      # tag without its v), .ShortCommit and .CommitDate (RFC 3339, UTC).
      - name: Build arguments, as GoReleaser stamps them
        id: stamp
        env:
          TAG: ${{ github.ref_name }}
        run: |
          {
            echo "version=${TAG#v}"
            echo "commit=$(git rev-parse --short HEAD)"
            echo "date=$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ)"
          } >> "$GITHUB_OUTPUT"
`,
	`      # No QEMU: the builder stage cross-compiles and the final stage runs
      # nothing, so linux/arm64 builds without emulation (ADR 0013).
      - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1
        with:
          # The BuildKit image, pinned by tag and multi-arch index digest.
          # renovate: datasource=docker depName=moby/buildkit versioning=semver
          driver-opts: image=moby/buildkit:v0.34.2@sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe
`,
	`      - uses: docker/login-action@8aa5831fb8d48d8f76594c7ddde61ae58019f550 # v4.8.0
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
`,
	`      # vX.Y.Z (the git tag) and X.Y, plus latest; a pre-release such as
      # v0.1.0-rc.1 gets its own tag only. The first entry decides latest.
      - name: Image tags
        id: tags
        uses: docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411302 # v6.2.0
        with:
          images: ghcr.io/timothymamo/eusov
          flavor: latest=auto
          tags: |
            type=semver,pattern={{raw}}
            type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}
`,
	`      # Labels come from the Dockerfile alone. BuildKit's own provenance and
      # SBOM are off: the attestations below replace them, signed (ADR 0014).
      - name: Build and push linux/amd64 and linux/arm64
        id: build
        uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.tags.outputs.tags }}
          annotations: index:org.opencontainers.image.source=https://github.com/timothymamo/eu-sov-cloud
          provenance: false
          sbom: false
          build-args: |
            VERSION=${{ steps.stamp.outputs.version }}
            COMMIT=${{ steps.stamp.outputs.commit }}
            DATE=${{ steps.stamp.outputs.date }}
`,
	`      - uses: sigstore/cosign-installer@756fe2d284c7292c4e9b62c8a28f1e5629c59532 # v4.1.3
        with:
          # renovate: datasource=github-releases depName=sigstore/cosign
          cosign-release: v3.1.4
`,
	`      # Keyless, through the job's OIDC identity; by digest, never by tag.
      - name: Sign the image
        env:
          DIGEST: ${{ steps.build.outputs.digest }}
        run: cosign sign --yes "ghcr.io/timothymamo/eusov@${DIGEST}"
`,
	`      - uses: actions/attest-build-provenance@4d101475d8b20a2381f78447822ac1eab6504dd8 # v4.2.2
        with:
          subject-name: ghcr.io/timothymamo/eusov
          subject-digest: ${{ steps.build.outputs.digest }}
          push-to-registry: true
          create-storage-record: false # an organisation API; needs artifact-metadata: write
`,
	`      # The SBOM of the pushed index, read from the registry. Nothing is
      # uploaded to the run or the release: the attestation carries it.
      - uses: anchore/sbom-action@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c # v0.24.3
        with:
          image: registry:ghcr.io/timothymamo/eusov@${{ steps.build.outputs.digest }}
          format: spdx-json
          output-file: eusov.spdx.json
          upload-artifact: false
          upload-release-assets: false
          # renovate: datasource=github-releases depName=anchore/syft
          syft-version: v1.57.1
`,
	`      - uses: actions/attest-sbom@c604332985a26aa8cf1bdc465b92731239ec6b9e # v4.1.0
        with:
          subject-name: ghcr.io/timothymamo/eusov
          subject-digest: ${{ steps.build.outputs.digest }}
          sbom-path: eusov.spdx.json
          push-to-registry: true
`,
}

// releaseWorkflow is the shape of .github/workflows/release.yml the check
// accepts: the test job, the goreleaser job and the image job.
var releaseWorkflow = releaseWorkflowWithImage(imageJobSteps)

// releaseWorkflowWithImage returns the release workflow with the image job
// built from steps, so that a case can leave one out or swap two.
func releaseWorkflowWithImage(steps []string) string {
	return releaseWorkflowWithoutImage + imageJobHead + strings.Join(steps, "")
}

// imageStepIndex returns the index of the one step of steps that holds part.
func imageStepIndex(t *testing.T, steps []string, part string) int {
	t.Helper()

	found := -1

	for i, step := range steps {
		if !strings.Contains(step, part) {
			continue
		}

		if found >= 0 {
			t.Fatalf("more than one image step holds %q", part)
		}

		found = i
	}

	if found < 0 {
		t.Fatalf("no image step holds %q", part)
	}

	return found
}

// TestReleaseWorkflow pins what the release workflow may say. It is read as a
// closed schema: the top-level keys, the jobs, their keys and their steps are
// fixed, so no step runs repository code, sets the go command's environment or
// swaps the goreleaser document before goreleaser runs, and the image job
// publishes only the signed image, provenance and SBOM that the check lists.
// The probes are the steps the second review and audit of issue #40 found the
// lexical check let through.
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

	// inImage replaces old with new in the image job alone. It fails unless the
	// image job holds old exactly once, so that a case that mistypes the fixture
	// text cannot pass for the wrong reason.
	inImage := func(old, new string) string {
		t.Helper()

		job := imageJobHead + strings.Join(imageJobSteps, "")
		if got := strings.Count(job, old); got != 1 {
			t.Fatalf("the fixture's image job holds %q %d times, want exactly once", old, got)
		}

		return releaseWorkflowWithoutImage + strings.Replace(job, old, new, 1)
	}

	// beforeImageStep inserts step, a sequence entry without its dash, before
	// the image job's step whose text starts with head.
	beforeImageStep := func(head, step string) string {
		t.Helper()

		return inImage(head, "      - "+step+"\n"+head)
	}

	// withoutImageStep returns the image job's steps without the one holding
	// part.
	withoutImageStep := func(part string) []string {
		t.Helper()

		i := imageStepIndex(t, imageJobSteps, part)

		steps := make([]string, 0, len(imageJobSteps))
		steps = append(steps, imageJobSteps[:i]...)
		steps = append(steps, imageJobSteps[i+1:]...)

		return steps
	}

	// swappedImageSteps returns the image job's steps with the two holding a and
	// b swapped.
	swappedImageSteps := func(a, b string) []string {
		t.Helper()

		steps := slices.Clone(imageJobSteps)
		ia := imageStepIndex(t, steps, a)
		ib := imageStepIndex(t, steps, b)
		steps[ia], steps[ib] = steps[ib], steps[ia]

		return steps
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
			// Every pin a Renovate update changes is matched by form: the actions'
			// commits, goreleaser's, cosign's and syft's versions, and the BuildKit
			// image in the image job's driver-opts.
			name: "a newer pin of every action and tool",
			workflow: strings.NewReplacer(
				"a4f02ab5355a589de60d917a659ce617ed19b3de", "0123456789abcdef0123456789abcdef01234567",
				"c3c9e263c25d99ce0380d002d59b67737d91b0dc", "1123456789abcdef0123456789abcdef01234567",
				`"v2.20.1"`, `"v2.21.0"`,
				"cosign-release: v3.1.4", "cosign-release: v3.2.0",
				"syft-version: v1.57.1", "syft-version: v1.58.0",
				"image=moby/buildkit:v0.34.2@sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe",
				"image=moby/buildkit:v0.35.0@sha256:1111111111111111111111111111111111111111111111111111111111111111",
			).Replace(releaseWorkflow),
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
		{name: "setup-go caching in the test job", workflow: strings.Replace(releaseWorkflow, "          cache: false # see the comment on the goreleaser job\n      - name: Test", "      - name: Test", 1), wantIn: `jobs: test: steps: step 2`},
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
		{name: "a fourth job", workflow: releaseWorkflow + "  other:\n    runs-on: ubuntu-latest\n", wantIn: `key "other"`},
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

		// The image job of issue #72 (ADR 0014): a third job of its own, after
		// the tests, that publishes the signed image, its provenance and its
		// SBOM to GHCR.
		{name: "no image job", workflow: releaseWorkflowWithoutImage, wantIn: `no "image"`},
		{name: "the image job on another runner", workflow: inImage("    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-24.04\n"), wantIn: "runs-on"},
		{name: "the image job without needs", workflow: inImage("    needs: test\n", ""), wantIn: `no "needs"`},
		{name: "the image job needing goreleaser", workflow: inImage("    needs: test\n", "    needs: goreleaser\n"), wantIn: "needs"},
		{name: "a job-level env on the image job", workflow: inImage("    needs: test\n", "    needs: test\n    env:\n      SNEAKY: x\n"), wantIn: `key "env"`},
		{name: "a job-level if on the image job", workflow: inImage("    needs: test\n", "    needs: test\n    if: true\n"), wantIn: `key "if"`},
		{name: "a container on the image job", workflow: inImage("    needs: test\n", "    needs: test\n    container: golang\n"), wantIn: `key "container"`},

		// The privileges stay split: the image job holds exactly contents: read,
		// packages: write, id-token: write and attestations: write, the
		// goreleaser job contents: write alone, and the workflow none.
		{name: "contents: write for the image job", workflow: inImage("      contents: read # check out the repository\n", "      contents: write # check out the repository\n"), wantIn: "contents"},
		{name: "an extra image permission", workflow: inImage("      attestations: write # store the provenance and SBOM attestations in the repository\n", "      attestations: write # store the provenance and SBOM attestations in the repository\n      artifact-metadata: write # an organisation API\n"), wantIn: "artifact-metadata"},
		{name: "packages: write for goreleaser too", workflow: strings.Replace(releaseWorkflow, "      contents: write # create the GitHub release and upload artifacts\n", "      contents: write # create the GitHub release and upload artifacts\n      packages: write # push the image\n", 1), wantIn: "packages"},
		{name: "id-token: write for goreleaser too", workflow: strings.Replace(releaseWorkflow, "      contents: write # create the GitHub release and upload artifacts\n", "      contents: write # create the GitHub release and upload artifacts\n      id-token: write # sign\n", 1), wantIn: "id-token"},

		// The image job's steps are a closed list too.
		{name: "an extra step in the image job", workflow: beforeImageStep("      - uses: sigstore/cosign-installer@", "run: ./scripts/prepare.sh"), wantIn: "steps"},
		{name: "a missing step in the image job", workflow: releaseWorkflowWithImage(withoutImageStep("docker/login-action@")), wantIn: "steps"},
		{name: "two image steps swapped", workflow: releaseWorkflowWithImage(swappedImageSteps("actions/attest-build-provenance@", "name: Sign the image")), wantIn: "steps"},
		{name: "build-push-action pinned by tag", workflow: inImage("docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc", "docker/build-push-action@v7"), wantIn: "build-push-action@v7"},
		{name: "setup-buildx-action pinned by tag", workflow: inImage("docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069", "docker/setup-buildx-action@v4"), wantIn: "setup-buildx-action@v4"},
		{name: "cosign-installer pinned by tag", workflow: inImage("sigstore/cosign-installer@756fe2d284c7292c4e9b62c8a28f1e5629c59532", "sigstore/cosign-installer@v4"), wantIn: "cosign-installer@v4"},

		// The build: two platforms, pushing, no cache in, BuildKit's own
		// attestations off, and nothing but the with: keys the check lists.
		{name: "a cache-from on the build", workflow: inImage("          push: true\n", "          push: true\n          cache-from: type=gha\n"), wantIn: "cache-from"},
		{name: "a cache-to on the build", workflow: inImage("          push: true\n", "          push: true\n          cache-to: type=gha,mode=max\n"), wantIn: "cache-to"},
		{name: "labels from metadata-action", workflow: inImage("          push: true\n", "          push: true\n          labels: ${{ steps.tags.outputs.labels }}\n"), wantIn: "labels"},
		{name: "push: false", workflow: inImage("          push: true\n", "          push: false\n"), wantIn: "push"},
		{name: "one platform", workflow: inImage("          platforms: linux/amd64,linux/arm64\n", "          platforms: linux/amd64\n"), wantIn: "platforms"},
		{name: "a third platform", workflow: inImage("          platforms: linux/amd64,linux/arm64\n", "          platforms: linux/amd64,linux/arm64,linux/arm/v7\n"), wantIn: "platforms"},
		{name: "BuildKit's provenance on", workflow: inImage("          provenance: false\n", "          provenance: true\n"), wantIn: "provenance"},
		{name: "BuildKit's SBOM on", workflow: inImage("          sbom: false\n", "          sbom: true\n"), wantIn: "sbom"},

		// The pins Renovate updates, by form: an exact release, an exact
		// syft, the BuildKit image by tag and multi-arch index digest.
		{name: "a cosign release range", workflow: inImage("cosign-release: v3.1.4", "cosign-release: v3"), wantIn: "cosign-release"},
		{name: "a syft version range", workflow: inImage("syft-version: v1.57.1", "syft-version: ~> v1"), wantIn: "syft-version"},
		{name: "a BuildKit tag without a digest", workflow: inImage("image=moby/buildkit:v0.34.2@sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe", "image=moby/buildkit:v0.34.2"), wantIn: "driver-opts"},
		{name: "a BuildKit tag that is not an exact release", workflow: inImage("image=moby/buildkit:v0.34.2@sha256:", "image=moby/buildkit:v0.34@sha256:"), wantIn: "driver-opts"},
		{name: "a BuildKit digest that is not 64 hex", workflow: inImage("sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe", "sha256:c5f8862f"), wantIn: "driver-opts"},

		// Signed and attested by digest, by the image job's own OIDC identity,
		// with tag-derived values through env: and never ${{ }} in a run:.
		{name: "the sign step by tag", workflow: inImage(`run: cosign sign --yes "ghcr.io/timothymamo/eusov@${DIGEST}"`, `run: cosign sign --yes "ghcr.io/timothymamo/eusov:${TAG}"`), wantIn: "run"},
		{name: "an interpolation inside a run script", workflow: inImage("            echo \"version=${TAG#v}\"\n", "            echo \"version=${{ github.ref_name }}\"\n"), wantIn: "run"},
		{name: "provenance not pushed to the registry", workflow: inImage("          subject-digest: ${{ steps.build.outputs.digest }}\n          push-to-registry: true\n", "          subject-digest: ${{ steps.build.outputs.digest }}\n          push-to-registry: false\n"), wantIn: "push-to-registry"},
		{name: "the SBOM attestation not pushed to the registry", workflow: inImage("          sbom-path: eusov.spdx.json\n          push-to-registry: true\n", "          sbom-path: eusov.spdx.json\n          push-to-registry: false\n"), wantIn: "push-to-registry"},
		{name: "a provenance attestation of another subject", workflow: inImage("          subject-name: ghcr.io/timothymamo/eusov\n          subject-digest: ${{ steps.build.outputs.digest }}\n          push-to-registry: true\n", "          subject-name: ghcr.io/example/eusov\n          subject-digest: ${{ steps.build.outputs.digest }}\n          push-to-registry: true\n"), wantIn: "subject-name"},
		{name: "an SBOM attestation of another digest", workflow: inImage("          subject-digest: ${{ steps.build.outputs.digest }}\n          sbom-path: eusov.spdx.json\n", "          subject-digest: ${{ github.sha }}\n          sbom-path: eusov.spdx.json\n"), wantIn: "subject-digest"},
		{name: "an attestation stored in the repository", workflow: inImage("          create-storage-record: false # an organisation API; needs artifact-metadata: write\n", "          create-storage-record: true\n"), wantIn: "create-storage-record"},

		// The SBOM: read from the registry by digest, uploaded nowhere.
		{name: "the SBOM read by tag", workflow: inImage("image: registry:ghcr.io/timothymamo/eusov@${{ steps.build.outputs.digest }}", "image: registry:ghcr.io/timothymamo/eusov:latest"), wantIn: "with: image"},
		{name: "the SBOM uploaded to the run", workflow: inImage("          upload-artifact: false\n", "          upload-artifact: true\n"), wantIn: "upload-artifact"},
		{name: "the SBOM uploaded to the release", workflow: inImage("          upload-release-assets: false\n", "          upload-release-assets: true\n"), wantIn: "upload-release-assets"},

		// The forbidden names the check refuses anywhere, the image job too.
		{name: "GOFLAGS on the image job", workflow: inImage("    needs: test\n", "    needs: test\n    env:\n      GOFLAGS: -tags=sneaky\n"), wantIn: "GOFLAGS"},
		{name: "GOFLAGS on the sign step", workflow: inImage("          DIGEST: ${{ steps.build.outputs.digest }}\n", "          DIGEST: ${{ steps.build.outputs.digest }}\n          GOFLAGS: -tags=sneaky\n"), wantIn: "GOFLAGS"},
		{name: "a go env file in the image job", workflow: inImage("            echo \"commit=$(git rev-parse --short HEAD)\"\n", "            echo \"commit=$(git rev-parse --short HEAD)\" >> \"${{ github.env }}\"\n"), wantIn: "github.env"},

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
