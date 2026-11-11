package check_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// The fixture below is .github/workflows/release.yml in parts: the top level,
// then the five jobs, each with the comments and keys above its steps
// (releaseJob.head) and its steps in order. Rendering it rebuilds the file byte
// for byte, so a case changes one part and leaves the rest exactly as the
// release has it, comments included: the check must tolerate a full-line
// comment above a job, between two steps, inside a with: mapping and on a
// value.

// releaseWorkflowTop is .github/workflows/release.yml up to its jobs:.
const releaseWorkflowTop = `name: Release

on:
  push:
    tags: ["v*.*.*"]

# No permission by default; each job asks for its own.
permissions: {}

# The steps of all five jobs are a closed list that test/architecture holds
# (check.ReleaseWorkflow): add no step, run: or env: here without changing it.
jobs:
`

// testJobHead is the test job up to its steps:.
const testJobHead = `  test:
    name: Test
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
`

// testSteps are the test job's steps: the whole history with every branch, the
// guard that the tagged commit is on main (ADR 0014), setup-go without the
// Actions cache, and make test.
var testSteps = []string{
	`      # The whole history, with every branch: the guard below needs origin/main.
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
`,
	`      # Only a commit on main is released (ADR 0014). Every other job needs
      # this one, so a tag on any other commit publishes nothing.
      - name: The tagged commit is on main
        run: |
          git rev-parse --verify --quiet refs/remotes/origin/main > /dev/null || {
            echo "origin/main is missing from the checkout" >&2
            exit 1
          }
          git merge-base --is-ancestor "$GITHUB_SHA" refs/remotes/origin/main || {
            echo "$GITHUB_SHA is not on main: nothing is released" >&2
            exit 1
          }
`,
	`      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: go.mod
          cache: false # see the comment on the goreleaser job
`,
	`      - name: Test
        run: make test
`,
}

// goreleaserJobHead is the goreleaser job up to its steps:.
const goreleaserJobHead = `  # A job of its own, on a fresh runner: no go env file and no environment or
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
`

// goreleaserSteps are the goreleaser job's steps: no repository code runs
// before goreleaser, which is asked for exactly release --clean.
var goreleaserSteps = []string{
	`      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
`,
	`      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: go.mod
          cache: false
`,
	`      - uses: goreleaser/goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de # v7.3.0
        with:
          distribution: goreleaser
          version: "v2.20.1" # exact pin; Renovate updates it (github-actions manager)
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
`,
}

// imageJobHead is the image job up to its steps: its digest output, the two
// permissions it needs to push, and no OIDC token.
const imageJobHead = `  # Builds and pushes the container image to GHCR (ADR 0014), after the GitHub
  # release: a failed GoReleaser publishes no image. It can push, nothing more:
  # no OIDC token, so nothing in it can sign. It builds from the Dockerfile with
  # the build arguments GoReleaser stamps into the release binaries, and
  # carries no build cache in: BuildKit starts empty on a fresh runner.
  image:
    name: Container image
    needs: [test, goreleaser]
    runs-on: ubuntu-latest
    permissions:
      contents: read # check out the repository
      packages: write # push the image to ghcr.io
    outputs:
      digest: ${{ steps.build.outputs.digest }}
    steps:
`

// imageSteps are the image job's steps: the build arguments GoReleaser stamps
// and the latest decision, a build of two platforms from an empty BuildKit,
// pushed, without BuildKit's own provenance or SBOM, tagged by the stamp's
// latest.
var imageSteps = []string{
	`      # The whole history and every tag, as GoReleaser checks it out: git picks
      # the length of a short commit from the number of objects it holds.
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
`,
	`      # The values GoReleaser stamps into the release binaries: .Version (the
      # tag without its v), .ShortCommit and .CommitDate (RFC 3339, UTC). And
      # whether the tag is the highest release tag, pre-releases left out:
      # only that tag moves latest, so a patch to an older line does not.
      - name: Build arguments, as GoReleaser stamps them
        id: stamp
        env:
          TAG: ${{ github.ref_name }}
        run: |
          highest="$(git tag -l 'v*.*.*' | sed -nE '/^v[0-9]+\.[0-9]+\.[0-9]+$/p' | sort -V | tail -n 1)"
          latest=false
          if [ "$TAG" = "$highest" ]; then latest=true; fi
          {
            echo "version=${TAG#v}"
            echo "commit=$(git rev-parse --short HEAD)"
            echo "date=$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ)"
            echo "latest=$latest"
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
	`      # vX.Y.Z (the git tag) and X.Y, plus latest for the highest release tag;
      # a pre-release such as v0.1.0-rc.1 gets its own tag only.
      - name: Image tags
        id: tags
        uses: docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411302 # v6.2.0
        with:
          images: ghcr.io/timothymamo/eusov
          flavor: latest=${{ steps.stamp.outputs.latest }}
          tags: |
            type=semver,pattern={{raw}}
            type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}
`,
	`      # Labels come from the Dockerfile alone. BuildKit's own provenance and
      # SBOM are off: the sign job's attestations replace them (ADR 0014).
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
}

// sbomJobHead is the SBOM job up to its steps: read-only to the registry, no
// OIDC token, no way to push.
const sbomJobHead = `  # The SBOM of the pushed index, read from the registry by digest. syft is
  # third-party code it downloads, so this job can only read the registry:
  # it cannot push, and has no OIDC token to sign with. The SBOM leaves the job
  # as a workflow artifact for the sign job to attest.
  sbom:
    name: SBOM
    needs: image
    runs-on: ubuntu-latest
    permissions:
      packages: read # pull the image from ghcr.io, private until made public
    steps:
`

// sbomSteps are the SBOM job's steps: log in read-only, syft the pushed index
// by digest, and upload the SPDX document under the name the sign job
// downloads.
var sbomSteps = []string{
	`      - uses: docker/login-action@8aa5831fb8d48d8f76594c7ddde61ae58019f550 # v4.8.0
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
`,
	`      - uses: anchore/sbom-action@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c # v0.24.3
        with:
          image: registry:ghcr.io/timothymamo/eusov@${{ needs.image.outputs.digest }}
          format: spdx-json
          output-file: eusov.spdx.json
          upload-artifact: false
          upload-release-assets: false
          # renovate: datasource=github-releases depName=anchore/syft
          syft-version: v1.57.1
`,
	`      - uses: actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2
        with:
          name: sbom
          path: eusov.spdx.json
          if-no-files-found: error
          retention-days: 1
`,
}

// signJobHead is the sign job up to its steps: the only job holding the
// signing identity, and one that must check nothing out and run no docker/* or
// anchore action.
const signJobHead = `  # The only job holding the signing identity (id-token: write). It runs
  # sigstore's cosign and GitHub's own actions only: no checkout, no docker/*
  # or anchore action. Everything is signed and attested by digest.
  sign:
    name: Sign and attest
    needs: [image, sbom]
    runs-on: ubuntu-latest
    permissions:
      packages: write # push the signatures and attestations to ghcr.io
      id-token: write # GitHub OIDC token for keyless cosign and attestation signing
      attestations: write # store the provenance and SBOM attestations in the repository
    steps:
`

// signSteps are the sign job's steps: cosign by exact release, a login fed the
// token through env:, a recursive signature by digest, the provenance
// attestation, the SBOM artifact, and its attestation.
var signSteps = []string{
	`      - uses: sigstore/cosign-installer@756fe2d284c7292c4e9b62c8a28f1e5629c59532 # v4.1.3
        with:
          # renovate: datasource=github-releases depName=sigstore/cosign
          cosign-release: v3.1.4
`,
	`      # cosign writes the registry credential where actions/attest reads it.
      - name: Log in to ghcr.io
        env:
          ACTOR: ${{ github.actor }}
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: printf '%s' "$GITHUB_TOKEN" | cosign login ghcr.io --username "$ACTOR" --password-stdin
`,
	`      # Keyless, through the job's OIDC identity; the index and, recursively,
      # each platform's image, so a per-platform digest verifies too.
      - name: Sign the image
        env:
          DIGEST: ${{ needs.image.outputs.digest }}
        run: cosign sign --yes --recursive "ghcr.io/timothymamo/eusov@${DIGEST}"
`,
	`      # SLSA build provenance: actions/attest's default without sbom-path.
      - name: Attest the build provenance
        uses: actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6 # v4.2.2
        with:
          subject-name: ghcr.io/timothymamo/eusov
          subject-digest: ${{ needs.image.outputs.digest }}
          push-to-registry: true
          create-storage-record: false # an organisation API; needs artifact-metadata: write
`,
	`      - uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2
        with:
          name: sbom
`,
	`      - name: Attest the SBOM
        uses: actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6 # v4.2.2
        with:
          subject-name: ghcr.io/timothymamo/eusov
          subject-digest: ${{ needs.image.outputs.digest }}
          sbom-path: eusov.spdx.json
          push-to-registry: true
          create-storage-record: false
`,
}

// releaseJob is one job of the release workflow fixture: its key in jobs:, the
// comments and keys above its steps, and its steps in order.
type releaseJob struct {
	key   string
	head  string
	steps []string
}

// text returns the job as the release workflow writes it.
func (j releaseJob) text() string {
	return j.head + strings.Join(j.steps, "")
}

// releaseFixture is .github/workflows/release.yml in parts.
type releaseFixture struct {
	top  string
	jobs []releaseJob
}

// releaseWorkflowFixture returns .github/workflows/release.yml in parts.
func releaseWorkflowFixture() releaseFixture {
	return releaseFixture{
		top: releaseWorkflowTop,
		jobs: []releaseJob{
			{key: "test", head: testJobHead, steps: testSteps},
			{key: "goreleaser", head: goreleaserJobHead, steps: goreleaserSteps},
			{key: "image", head: imageJobHead, steps: imageSteps},
			{key: "sbom", head: sbomJobHead, steps: sbomSteps},
			{key: "sign", head: signJobHead, steps: signSteps},
		},
	}
}

// render returns the fixture as the release workflow file.
func (f releaseFixture) render() string {
	blocks := make([]string, 0, len(f.jobs))
	for _, j := range f.jobs {
		blocks = append(blocks, j.text())
	}

	return f.top + strings.Join(blocks, "\n")
}

// index returns the index of job key.
func (f releaseFixture) index(t *testing.T, key string) int {
	t.Helper()

	for i, j := range f.jobs {
		if j.key == key {
			return i
		}
	}

	t.Fatalf("the fixture holds no job %q", key)

	return -1
}

// withSteps returns the workflow with the steps of job key replaced by steps.
func (f releaseFixture) withSteps(t *testing.T, key string, steps []string) string {
	t.Helper()

	jobs := slices.Clone(f.jobs)
	jobs[f.index(t, key)].steps = steps

	return releaseFixture{top: f.top, jobs: jobs}.render()
}

// inJob returns the workflow with old replaced by new in the text of job key
// alone: its keys and steps, comments included. It fails unless that job holds
// old exactly once, so that a case that mistypes the fixture text cannot pass
// for the wrong reason.
func (f releaseFixture) inJob(t *testing.T, key, old, new string) string {
	t.Helper()

	jobs := slices.Clone(f.jobs)

	i := f.index(t, key)
	if got := strings.Count(jobs[i].text(), old); got != 1 {
		t.Fatalf("the fixture's %s job holds %q %d times, want exactly once", key, old, got)
	}

	jobs[i].head = strings.Replace(jobs[i].text(), old, new, 1)
	jobs[i].steps = nil

	return releaseFixture{top: f.top, jobs: jobs}.render()
}

// withoutJob returns the workflow without job key.
func (f releaseFixture) withoutJob(t *testing.T, key string) string {
	t.Helper()

	i := f.index(t, key)

	jobs := slices.Clone(f.jobs)
	jobs = append(jobs[:i], jobs[i+1:]...)

	return releaseFixture{top: f.top, jobs: jobs}.render()
}

// stepIndex returns the index of the one step of steps that holds part.
func stepIndex(t *testing.T, steps []string, part string) int {
	t.Helper()

	found := -1

	for i, step := range steps {
		if !strings.Contains(step, part) {
			continue
		}

		if found >= 0 {
			t.Fatalf("more than one step holds %q", part)
		}

		found = i
	}

	if found < 0 {
		t.Fatalf("no step holds %q", part)
	}

	return found
}

// withoutStep returns steps without the one holding part.
func withoutStep(t *testing.T, steps []string, part string) []string {
	t.Helper()

	i := stepIndex(t, steps, part)

	out := make([]string, 0, len(steps))
	out = append(out, steps[:i]...)
	out = append(out, steps[i+1:]...)

	return out
}

// swappedSteps returns steps with the two holding a and b swapped.
func swappedSteps(t *testing.T, steps []string, a, b string) []string {
	t.Helper()

	out := slices.Clone(steps)
	ia := stepIndex(t, out, a)
	ib := stepIndex(t, out, b)
	out[ia], out[ib] = out[ib], out[ia]

	return out
}

// beforeStep returns steps with step, a sequence entry without its dash,
// inserted before the one whose text holds head.
func beforeStep(t *testing.T, steps []string, head, step string) []string {
	t.Helper()

	i := stepIndex(t, steps, head)

	out := make([]string, 0, len(steps)+1)
	out = append(out, steps[:i]...)
	out = append(out, "      - "+step+"\n")
	out = append(out, steps[i:]...)

	return out
}

// TestReleaseWorkflow pins what the release workflow may say. It is read as a
// closed schema: the top-level keys, the five jobs, their keys and their steps
// are fixed, so no step runs repository code, sets the go command's environment
// or swaps the goreleaser document before goreleaser runs; only a commit on
// main is released; and the publishing privileges stay split over three jobs,
// so the image can push but not sign, the SBOM job can only read, and the sign
// job checks nothing out and runs sigstore's and GitHub's actions alone. The
// probes are the steps the reviews and audits of issues #40 and #72 found a
// lexical check let through.
func TestReleaseWorkflow(t *testing.T) {
	t.Parallel()

	release := releaseWorkflowFixture()
	releaseWorkflow := release.render()

	// beforeGoreleaser inserts step before the goreleaser step.
	beforeGoreleaser := func(step string) string {
		return strings.Replace(releaseWorkflow, "      - uses: goreleaser/goreleaser-action@",
			"      - "+step+"\n      - uses: goreleaser/goreleaser-action@", 1)
	}

	// inTestJob inserts step before the test job's make test.
	inTestJob := func(step string) string {
		return strings.Replace(releaseWorkflow, "      - name: Test\n", "      - "+step+"\n      - name: Test\n", 1)
	}

	// in replaces old with new in one job alone, failing unless that job holds
	// old exactly once.
	in := func(key, old, new string) string {
		return release.inJob(t, key, old, new)
	}

	// steps returns the workflow with the steps of one job replaced.
	steps := func(key string, steps []string) string {
		return release.withSteps(t, key, steps)
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
				"3d3c42e5aac5ba805825da76410c181273ba90b1", "0d3c42e5aac5ba805825da76410c181273ba90b1",
				"b7ad1dad31e06c5925ef5d2fc7ad053ef454303e", "07ad1dad31e06c5925ef5d2fc7ad053ef454303e",
				"a4f02ab5355a589de60d917a659ce617ed19b3de", "a4f02ab5355a589de60d917a659ce617ed19b3df",
				"f87e5991a6d7451dcb8d9637bfbc97413f497069", "087e5991a6d7451dcb8d9637bfbc97413f497069",
				"8aa5831fb8d48d8f76594c7ddde61ae58019f550", "0aa5831fb8d48d8f76594c7ddde61ae58019f550",
				"dc802804100637a589fabce1cb79ff13a1411302", "dc802804100637a589fabce1cb79ff13a1411303",
				"c3c9e263c25d99ce0380d002d59b67737d91b0dc", "03c9e263c25d99ce0380d002d59b67737d91b0dc",
				"756fe2d284c7292c4e9b62c8a28f1e5629c59532", "756fe2d284c7292c4e9b62c8a28f1e5629c59533",
				"1e69f48acb82d1966a394da916b4c1698aa569d6", "1e69f48acb82d1966a394da916b4c1698aa569d7",
				"66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c", "66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6d",
				"9000827ccba6bdab643e8b6fd33ac0654aef8333", "9000827ccba6bdab643e8b6fd33ac0654aef8334",
				"cf430e030ddbb5b0abf93d22962f4752f3646cd9", "cf430e030ddbb5b0abf93d22962f4752f3646cda",
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
		{name: "a step added to the test job", workflow: inTestJob(`run: go env -w GOFLAGS=-x`), wantIn: "holds 5 steps, want exactly 4"},
		{
			name:     "a step replaced in the goreleaser job",
			workflow: in("goreleaser", "      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0\n        with:\n          go-version-file: go.mod\n          cache: false\n", "      - run: ./scripts/prepare.sh\n"),
			wantIn:   "step 2",
		},
		{
			name:     "the test job running another command",
			workflow: in("test", "        run: make test\n", "        run: make test && go env -w GOFLAGS=-x\n"),
			wantIn:   "want make test: run is",
		},

		// The whole history and the guard that the tagged commit is on main
		// (ADR 0014): the second step of the test job, before anything runs
		// repository code, and exact.
		{name: "the test checkout without the full history", workflow: in("test", "          fetch-depth: 0\n", ""), wantIn: "fetch-depth"},
		{name: "no guard step", workflow: steps("test", withoutStep(t, testSteps, "name: The tagged commit is on main")), wantIn: "holds 3 steps, want exactly 4"},
		{name: "the guard after setup-go", workflow: steps("test", swappedSteps(t, testSteps, "name: The tagged commit is on main", "actions/setup-go@")), wantIn: "step 2"},
		{
			name:     "the guard without the ancestry check",
			workflow: in("test", `git merge-base --is-ancestor "$GITHUB_SHA" refs/remotes/origin/main || {`, `git rev-parse --verify --quiet "$GITHUB_SHA" > /dev/null || {`),
			wantIn:   "merge-base",
		},
		{
			name:     "the guard without the origin/main check",
			workflow: in("test", "git rev-parse --verify --quiet refs/remotes/origin/main > /dev/null", "git rev-parse --verify --quiet HEAD > /dev/null"),
			wantIn:   "refs/remotes/origin/main",
		},
		{
			name:     "an expression in the guard script",
			workflow: in("test", `git merge-base --is-ancestor "$GITHUB_SHA" refs/remotes/origin/main`, `git merge-base --is-ancestor "${{ github.sha }}" refs/remotes/origin/main`),
			wantIn:   "holds a ${{ }} expression",
		},
		{name: "an if on the guard", workflow: in("test", "      - name: The tagged commit is on main\n", "      - name: The tagged commit is on main\n        if: always()\n"), wantIn: `key "if"`},
		{name: "a shell on the guard", workflow: in("test", "      - name: The tagged commit is on main\n", "      - name: The tagged commit is on main\n        shell: bash\n"), wantIn: `key "shell"`},

		// setup-go's Actions cache (GOMODCACHE and GOCACHE) would carry
		// state from the test job, or from CI on main, into the goreleaser
		// job, so both jobs turn it off.
		{name: "setup-go caching in the test job", workflow: in("test", "          cache: false # see the comment on the goreleaser job\n", "          cache: true # see the comment on the goreleaser job\n"), wantIn: `jobs: test: steps: step 3`},
		{name: "setup-go caching in the goreleaser job", workflow: in("goreleaser", "          cache: false\n", "          cache: true\n"), wantIn: `jobs: goreleaser: steps: step 2`},
		{name: "setup-go without cache: false", workflow: in("goreleaser", "          cache: false\n", ""), wantIn: `no "cache"`},

		// The tokens through which a step sets the environment or the PATH of
		// the steps after it, wherever they are, comments included.
		{name: "github.path in a comment", workflow: strings.Replace(releaseWorkflow, "# v7.3.0", "# ${{ github.path }}", 1), wantIn: "github.path"},
		{name: "GITHUB.ENV in upper case", workflow: strings.Replace(releaseWorkflow, "# v7.3.0", "# ${{ GITHUB.ENV }}", 1), wantIn: "github.env"},
		{name: "GITHUB_ENV in a comment", workflow: strings.Replace(releaseWorkflow, "# v7.3.0", "# GITHUB_ENV", 1), wantIn: "GITHUB_ENV"},
		{name: "GOEXPERIMENT in a comment", workflow: strings.Replace(releaseWorkflow, "# v7.0.1", "# GOEXPERIMENT", 1), wantIn: "GOEXPERIMENT"},

		// The keys around the steps.
		{name: "a workflow-level env", workflow: strings.Replace(releaseWorkflow, "jobs:\n", "env:\n  SNEAKY: x\njobs:\n", 1), wantIn: `key "env"`},
		{name: "a workflow-level defaults", workflow: strings.Replace(releaseWorkflow, "jobs:\n", "defaults:\n  run:\n    shell: bash\njobs:\n", 1), wantIn: `key "defaults"`},
		{name: "a sixth job", workflow: releaseWorkflow + "  other:\n    runs-on: ubuntu-latest\n", wantIn: `key "other"`},
		{name: "a job-level env", workflow: in("image", "    needs: [test, goreleaser]\n", "    needs: [test, goreleaser]\n    env:\n      SNEAKY: x\n"), wantIn: `key "env"`},
		{name: "a container", workflow: in("image", "    needs: [test, goreleaser]\n", "    needs: [test, goreleaser]\n    container: golang\n"), wantIn: `key "container"`},
		{name: "no needs on goreleaser", workflow: in("goreleaser", "    needs: test\n", ""), wantIn: `no "needs"`},
		{name: "goreleaser needs another job", workflow: in("goreleaser", "    needs: test\n", "    needs: image\n"), wantIn: "needs"},
		{name: "write permission at the top", workflow: strings.Replace(releaseWorkflow, "permissions: {}", "permissions:\n  contents: write", 1), wantIn: "permissions"},
		{name: "write-all at the top", workflow: strings.Replace(releaseWorkflow, "permissions: {}", "permissions: write-all", 1), wantIn: "permissions"},
		{name: "write permission for the tests", workflow: in("test", "      contents: read\n", "      contents: write\n"), wantIn: "contents"},
		{name: "another trigger", workflow: strings.Replace(releaseWorkflow, "  push:\n", "  workflow_dispatch:\n  push:\n", 1), wantIn: "workflow_dispatch"},
		{name: "a step-level if", workflow: in("test", "        run: make test\n", "        run: make test\n        if: false\n"), wantIn: `key "if"`},
		{name: "a working-directory", workflow: in("test", "        run: make test\n", "        run: make test\n        working-directory: sub\n"), wantIn: `key "working-directory"`},
		{name: "an env on another step", workflow: in("goreleaser", "          cache: false\n", "          cache: false\n        env:\n          A: b\n"), wantIn: `key "env"`},
		{name: "a local action", workflow: in("test", "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e", "./.github/actions/setup"), wantIn: "uses"},
		{name: "a reusable workflow", workflow: in("goreleaser", "    runs-on: ubuntu-latest\n", "    uses: example/repo/.github/workflows/x.yml@main\n"), wantIn: "runs-on"},

		// What goreleaser is asked to do.
		{name: "an argument glued to --clean", workflow: in("goreleaser", "args: release --clean", "args: release --clean-f.goreleaser.prod.yaml"), wantIn: "args"},
		{name: "a --config", workflow: in("goreleaser", "args: release --clean", "args: release --clean --config x.yaml"), wantIn: "args"},
		{name: "no args", workflow: in("goreleaser", "          args: release --clean\n", ""), wantIn: `no "args"`},
		{name: "a workdir", workflow: in("goreleaser", "distribution: goreleaser", "distribution: goreleaser\n          workdir: sub"), wantIn: `key "workdir"`},
		{name: "a goreleaser version range", workflow: in("goreleaser", `"v2.20.1"`, `"~> v2"`), wantIn: "version"},
		{name: "an action pinned by tag", workflow: in("goreleaser", "goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de", "goreleaser-action@v7"), wantIn: "goreleaser-action@v7"},
		{name: "a second key in the goreleaser env", workflow: in("goreleaser", "          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n", "          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n          OTHER: x\n"), wantIn: "OTHER"},
		{name: "a GITHUB_TOKEN from somewhere else", workflow: in("goreleaser", "GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}", "GITHUB_TOKEN: ${{ vars.TOKEN }}"), wantIn: "GITHUB_TOKEN"},
		{name: "a flow-style env", workflow: in("goreleaser", "        env:\n          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n", "        env: {GITHUB_TOKEN: x, A: b}\n"), wantIn: "env"},
		{name: "a repeated key", workflow: in("goreleaser", "    needs: test\n", "    needs: test\n    needs: test\n"), wantIn: "repeated"},
		{name: "a tab", workflow: strings.Replace(releaseWorkflow, "# v7.0.1", "#\tv7.0.1", 1), wantIn: "U+0009"},

		// The jobs are a closed list: the image that publishes, the SBOM that
		// reads, the sign job that signs.
		{name: "no image job", workflow: release.withoutJob(t, "image"), wantIn: `no "image"`},
		{name: "no sbom job", workflow: release.withoutJob(t, "sbom"), wantIn: `no "sbom"`},
		{name: "no sign job", workflow: release.withoutJob(t, "sign"), wantIn: `no "sign"`},

		// Publishing waits for the release: the image job needs the tests and
		// goreleaser, the SBOM job the image, the sign job both the image and
		// its SBOM.
		{name: "the image job needing only the tests", workflow: in("image", "    needs: [test, goreleaser]\n", "    needs: [test]\n"), wantIn: "needs"},
		{name: "the image job needing goreleaser first", workflow: in("image", "    needs: [test, goreleaser]\n", "    needs: [goreleaser, test]\n"), wantIn: "needs"},
		{name: "the sbom job needing the sign job", workflow: in("sbom", "    needs: image\n", "    needs: sign\n"), wantIn: "needs"},
		{name: "the sign job needing only the image", workflow: in("sign", "    needs: [image, sbom]\n", "    needs: [image]\n"), wantIn: "needs"},
		{name: "the sign job needing the tests too", workflow: in("sign", "    needs: [image, sbom]\n", "    needs: [image, sbom, test]\n"), wantIn: "needs"},

		// The image job publishes and nothing more: contents: read, packages:
		// write, and no OIDC token or attestation scope, so nothing in it can
		// sign.
		{name: "the image job on another runner", workflow: in("image", "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-24.04\n"), wantIn: "runs-on"},
		{name: "the image job without needs", workflow: in("image", "    needs: [test, goreleaser]\n", ""), wantIn: `no "needs"`},
		{name: "a job-level if on the image job", workflow: in("image", "    needs: [test, goreleaser]\n", "    needs: [test, goreleaser]\n    if: true\n"), wantIn: `key "if"`},
		{name: "contents: write for the image job", workflow: in("image", "      contents: read # check out the repository\n", "      contents: write # check out the repository\n"), wantIn: "contents"},
		{name: "id-token: write for the image job", workflow: in("image", "      packages: write # push the image to ghcr.io\n", "      packages: write # push the image to ghcr.io\n      id-token: write # sign\n"), wantIn: "id-token"},
		{name: "attestations: write for the image job", workflow: in("image", "      packages: write # push the image to ghcr.io\n", "      packages: write # push the image to ghcr.io\n      attestations: write # store them\n"), wantIn: "attestations"},
		{name: "an extra image permission", workflow: in("image", "      packages: write # push the image to ghcr.io\n", "      packages: write # push the image to ghcr.io\n      artifact-metadata: write # an organisation API\n"), wantIn: "artifact-metadata"},
		{name: "no packages: write for the image job", workflow: in("image", "      packages: write # push the image to ghcr.io\n", ""), wantIn: "packages"},

		// The goreleaser job keeps contents: write alone.
		{name: "packages: write for goreleaser too", workflow: in("goreleaser", "      contents: write # create the GitHub release and upload artifacts\n", "      contents: write # create the GitHub release and upload artifacts\n      packages: write # push the image\n"), wantIn: "packages"},
		{name: "id-token: write for goreleaser too", workflow: in("goreleaser", "      contents: write # create the GitHub release and upload artifacts\n", "      contents: write # create the GitHub release and upload artifacts\n      id-token: write # sign\n"), wantIn: "id-token"},
		{name: "no contents: write for goreleaser", workflow: in("goreleaser", "      contents: write # create the GitHub release and upload artifacts\n", ""), wantIn: "contents"},

		// The SBOM job only reads the registry: syft is third-party code, so
		// packages: read alone and no OIDC token.
		{name: "the sbom job on another runner", workflow: in("sbom", "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-24.04\n"), wantIn: "runs-on"},
		{name: "the sbom job without needs", workflow: in("sbom", "    needs: image\n", ""), wantIn: `no "needs"`},
		{name: "packages: write for the sbom job", workflow: in("sbom", "      packages: read # pull the image from ghcr.io, private until made public\n", "      packages: write # pull the image from ghcr.io, private until made public\n"), wantIn: "packages"},
		{name: "contents: read for the sbom job", workflow: in("sbom", "      packages: read # pull the image from ghcr.io, private until made public\n", "      contents: read # a checkout\n      packages: read # pull the image from ghcr.io, private until made public\n"), wantIn: "contents"},
		{name: "id-token: write for the sbom job", workflow: in("sbom", "      packages: read # pull the image from ghcr.io, private until made public\n", "      packages: read # pull the image from ghcr.io, private until made public\n      id-token: write # sign\n"), wantIn: "id-token"},

		// The image job's steps are a closed list too.
		{name: "an extra step in the image job", workflow: steps("image", beforeStep(t, imageSteps, "      - uses: docker/setup-buildx-action@", "run: ./scripts/prepare.sh")), wantIn: "holds 7 steps, want exactly 6"},
		{name: "a missing step in the image job", workflow: steps("image", withoutStep(t, imageSteps, "docker/login-action@")), wantIn: "holds 5 steps, want exactly 6"},
		{name: "two image steps swapped", workflow: steps("image", swappedSteps(t, imageSteps, "docker/setup-buildx-action@", "docker/login-action@")), wantIn: "step 3"},
		{name: "build-push-action pinned by tag", workflow: in("image", "docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc", "docker/build-push-action@v7"), wantIn: "build-push-action@v7"},
		{name: "setup-buildx-action pinned by tag", workflow: in("image", "docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069", "docker/setup-buildx-action@v4"), wantIn: "setup-buildx-action@v4"},
		{name: "metadata-action pinned by tag", workflow: in("image", "docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411302", "docker/metadata-action@v6"), wantIn: "metadata-action@v6"},
		{name: "login-action pinned by tag", workflow: in("image", "docker/login-action@8aa5831fb8d48d8f76594c7ddde61ae58019f550", "docker/login-action@v4"), wantIn: "login-action@v4"},

		// The image job's digest output: what the SBOM job reads the image by
		// and the sign job signs and attests.
		{name: "the image job without its outputs", workflow: in("image", "    outputs:\n      digest: ${{ steps.build.outputs.digest }}\n", ""), wantIn: `no "outputs"`},
		{name: "a second output on the image job", workflow: in("image", "      digest: ${{ steps.build.outputs.digest }}\n", "      digest: ${{ steps.build.outputs.digest }}\n      url: https://example.com\n"), wantIn: "outputs"},
		{name: "another digest output", workflow: in("image", "      digest: ${{ steps.build.outputs.digest }}\n", "      digest: ${{ steps.build.outputs.imageid }}\n"), wantIn: "digest"},

		// The build: two platforms, pushing, no cache in, BuildKit's own
		// attestations off, and nothing but the with: keys the check lists.
		{name: "a cache-from on the build", workflow: in("image", "          push: true\n", "          push: true\n          cache-from: type=gha\n"), wantIn: "cache-from"},
		{name: "a cache-to on the build", workflow: in("image", "          push: true\n", "          push: true\n          cache-to: type=gha,mode=max\n"), wantIn: "cache-to"},
		{name: "labels from metadata-action", workflow: in("image", "          push: true\n", "          push: true\n          labels: ${{ steps.tags.outputs.labels }}\n"), wantIn: "labels"},
		{name: "push: false", workflow: in("image", "          push: true\n", "          push: false\n"), wantIn: "push"},
		{name: "one platform", workflow: in("image", "          platforms: linux/amd64,linux/arm64\n", "          platforms: linux/amd64\n"), wantIn: "platforms"},
		{name: "a third platform", workflow: in("image", "          platforms: linux/amd64,linux/arm64\n", "          platforms: linux/amd64,linux/arm64,linux/arm/v7\n"), wantIn: "platforms"},
		{name: "another source annotation", workflow: in("image", "          annotations: index:org.opencontainers.image.source=https://github.com/timothymamo/eu-sov-cloud\n", "          annotations: index:org.opencontainers.image.source=https://example.com\n"), wantIn: "annotations"},
		{name: "BuildKit's provenance on", workflow: in("image", "          provenance: false\n", "          provenance: true\n"), wantIn: "with: provenance"},
		{name: "BuildKit's SBOM on", workflow: in("image", "          sbom: false\n", "          sbom: true\n"), wantIn: "with: sbom"},
		{name: "a build argument from git", workflow: in("image", "            VERSION=${{ steps.stamp.outputs.version }}\n", "            VERSION=$(git describe --tags)\n"), wantIn: "build-args"},

		// The stamp: the arguments GoReleaser stamps, the tag through env:, and
		// the latest decision from the highest release tag.
		{name: "an extra stamp env", workflow: in("image", "          TAG: ${{ github.ref_name }}\n", "          TAG: ${{ github.ref_name }}\n          REF: ${{ github.ref }}\n"), wantIn: "env holds"},
		{name: "a stamp without the latest decision", workflow: in("image", "\n          if [ \"$TAG\" = \"$highest\" ]; then latest=true; fi", ""), wantIn: "run is"},
		{name: "a stamp with latest always", workflow: in("image", `echo "latest=$latest"`, `echo "latest=true"`), wantIn: "latest=$latest"},
		{name: "a stamp that does not sort the tags by version", workflow: in("image", `| sed -nE '/^v[0-9]+\.[0-9]+\.[0-9]+$/p'`, ""), wantIn: "sed -nE"},
		{
			name:     "an interpolation inside a run script",
			workflow: in("image", `            echo "version=${TAG#v}"`+"\n", `            echo "version=${{ github.ref_name }}"`+"\n"),
			wantIn:   "holds a ${{ }} expression",
		},
		{name: "an extra line in the stamp script", workflow: in("image", "          } >> \"$GITHUB_OUTPUT\"\n", "          } >> \"$GITHUB_OUTPUT\"\n          echo sneaky=1\n"), wantIn: "run is"},
		{name: "a stamp script in a chomped block", workflow: in("image", "        run: |\n", "        run: |-\n"), wantIn: "run has lines below it, want one value on its line"},
		{name: "a stamp script in a folded block", workflow: in("image", "        run: |\n", "        run: >\n"), wantIn: "run has lines below it, want one value on its line"},
		{name: "a stamp script line indented less than its first", workflow: in("image", "          } >> \"$GITHUB_OUTPUT\"\n", "          } >> \"$GITHUB_OUTPUT\"\n         ; echo sneaky=1\n"), wantIn: "a line of run indented less than its first"},
		{name: "a comment line before the stamp script's first line", workflow: in("image", "        run: |\n", "        run: |\n          # a comment the reader must not treat as one\n"), wantIn: "run has a line before its first content line"},

		// The pins Renovate updates, by form: an exact release, an exact syft,
		// the BuildKit image by tag and multi-arch index digest.
		{name: "a BuildKit tag without a digest", workflow: in("image", "image=moby/buildkit:v0.34.2@sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe", "image=moby/buildkit:v0.34.2"), wantIn: "driver-opts"},
		{name: "a BuildKit tag that is not an exact release", workflow: in("image", "image=moby/buildkit:v0.34.2@sha256:", "image=moby/buildkit:v0.34@sha256:"), wantIn: "driver-opts"},
		{name: "a BuildKit digest that is not 64 hex", workflow: in("image", "sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe", "sha256:c5f8862f"), wantIn: "driver-opts"},

		// Another registry, another image name: the checks below read the one
		// the release publishes.
		{name: "another registry", workflow: in("image", "          registry: ghcr.io\n", "          registry: docker.io\n"), wantIn: "registry"},
		{name: "another user", workflow: in("image", "          username: ${{ github.actor }}\n", "          username: someone\n"), wantIn: "username"},
		{name: "another image name", workflow: in("image", "          images: ghcr.io/timothymamo/eusov\n", "          images: ghcr.io/example/eusov\n"), wantIn: "images"},

		// latest comes from the stamp step's output alone: no literal, no
		// other step, no latest=auto.
		{name: "latest from another step", workflow: in("image", "          flavor: latest=${{ steps.stamp.outputs.latest }}\n", "          flavor: latest=${{ steps.latest.outputs.highest }}\n"), wantIn: "flavor"},
		{name: "latest always", workflow: in("image", "          flavor: latest=${{ steps.stamp.outputs.latest }}\n", "          flavor: latest=true\n"), wantIn: "flavor"},
		{name: "latest=auto", workflow: in("image", "          flavor: latest=${{ steps.stamp.outputs.latest }}\n", "          flavor: latest=auto\n"), wantIn: "flavor"},

		// The literal blocks the image job's tags, build arguments and stamp
		// script are: a line a YAML reader keeps as content (a # line, a blank
		// line, a line after the last one) is part of the value, and a block form
		// the reader does not follow (chomped |-, folded >, a line indented less
		// than the first) is refused, so none of them slips a line past the exact
		// value the check lists.
		{name: "a comment line inside the tags block", workflow: in("image", "            type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}\n", "            # a comment the reader must not treat as one\n            type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}\n"), wantIn: "with: tags"},
		{name: "a comment line after the build arguments", workflow: in("image", "            DATE=${{ steps.stamp.outputs.date }}\n", "            DATE=${{ steps.stamp.outputs.date }}\n            # a comment the reader must not treat as one\n"), wantIn: "with: build-args"},
		{name: "a comment line after the tags block content", workflow: in("image", "            type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}\n", "            type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}\n            # a trailing note\n"), wantIn: "with: tags"},
		{name: "a blank line inside the tags block", workflow: in("image", "            type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}\n", "\n            type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}\n"), wantIn: "with: tags"},
		{name: "another tag rule", workflow: in("image", "            type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}\n", "            type=semver,pattern={{major}}\n"), wantIn: "with: tags"},

		// The SBOM job's steps: log in read-only, the pushed index by digest,
		// and the SPDX document under the name the sign job downloads.
		{name: "an extra step in the sbom job", workflow: steps("sbom", beforeStep(t, sbomSteps, "      - uses: anchore/sbom-action@", "run: ./scripts/prepare.sh")), wantIn: "holds 4 steps, want exactly 3"},
		{name: "a missing step in the sbom job", workflow: steps("sbom", withoutStep(t, sbomSteps, "actions/upload-artifact@")), wantIn: "holds 2 steps, want exactly 3"},
		{name: "two sbom steps swapped", workflow: steps("sbom", swappedSteps(t, sbomSteps, "docker/login-action@", "anchore/sbom-action@")), wantIn: "step 1"},
		{name: "the SBOM read by tag", workflow: in("sbom", "image: registry:ghcr.io/timothymamo/eusov@${{ needs.image.outputs.digest }}", "image: registry:ghcr.io/timothymamo/eusov:latest"), wantIn: "with: image"},
		{name: "another SBOM format", workflow: in("sbom", "          format: spdx-json\n", "          format: cyclonedx-json\n"), wantIn: "format"},
		{name: "the SBOM uploaded to the run", workflow: in("sbom", "          upload-artifact: false\n", "          upload-artifact: true\n"), wantIn: "upload-artifact"},
		{name: "the SBOM uploaded to the release", workflow: in("sbom", "          upload-release-assets: false\n", "          upload-release-assets: true\n"), wantIn: "upload-release-assets"},
		{name: "a syft version range", workflow: in("sbom", "syft-version: v1.57.1", "syft-version: ~> v1"), wantIn: "syft-version"},
		{name: "another SBOM artifact name", workflow: in("sbom", "          name: sbom\n", "          name: sboms\n"), wantIn: "name"},
		{name: "another SBOM artifact path", workflow: in("sbom", "          path: eusov.spdx.json\n", "          path: report.json\n"), wantIn: "path"},
		{name: "a tolerated missing SBOM", workflow: in("sbom", "          if-no-files-found: error\n", "          if-no-files-found: warn\n"), wantIn: "if-no-files-found"},
		{name: "a longer SBOM retention", workflow: in("sbom", "          retention-days: 1\n", "          retention-days: 7\n"), wantIn: "retention-days"},
		{name: "sbom-action pinned by tag", workflow: in("sbom", "anchore/sbom-action@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c", "anchore/sbom-action@v0"), wantIn: "sbom-action@v0"},

		// The sign job: the signing identity, with GitHub's and sigstore's code
		// alone, no checkout, and everything by digest.
		{name: "the sign job on another runner", workflow: in("sign", "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-24.04\n"), wantIn: "runs-on"},
		{name: "the sign job without needs", workflow: in("sign", "    needs: [image, sbom]\n", ""), wantIn: `no "needs"`},
		{name: "an extra step in the sign job", workflow: steps("sign", beforeStep(t, signSteps, "      - uses: actions/download-artifact@", "uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1")), wantIn: "holds 7 steps, want exactly 6"},
		{name: "a checkout in place of the cosign installer", workflow: steps("sign", slices.Replace(slices.Clone(signSteps), 0, 1, "      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1\n")), wantIn: "cosign-installer"},
		{name: "a docker login in place of the cosign login", workflow: steps("sign", slices.Replace(slices.Clone(signSteps), 1, 2, "      - uses: docker/login-action@8aa5831fb8d48d8f76594c7ddde61ae58019f550 # v4.8.0\n        with:\n          registry: ghcr.io\n")), wantIn: "step 2"},
		{name: "an anchore action in place of the provenance attestation", workflow: steps("sign", slices.Replace(slices.Clone(signSteps), 3, 4, "      - uses: anchore/sbom-action@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c # v0.24.3\n")), wantIn: "actions/attest"},
		{name: "two sign steps swapped", workflow: steps("sign", swappedSteps(t, signSteps, "name: Attest the build provenance", "actions/download-artifact@")), wantIn: "step 4"},
		{name: "an upload in place of the artifact download", workflow: in("sign", "      - uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2\n", "      - uses: actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2\n"), wantIn: "download-artifact"},
		{name: "a cosign installer pinned by tag", workflow: in("sign", "sigstore/cosign-installer@756fe2d284c7292c4e9b62c8a28f1e5629c59532", "sigstore/cosign-installer@v4"), wantIn: "cosign-installer@v4"},
		{name: "a cosign release range", workflow: in("sign", "cosign-release: v3.1.4", "cosign-release: v3"), wantIn: "cosign-release"},

		// The login: the token and the actor through env:, never pasted into
		// the script, and nothing else in the environment.
		{name: "an extra env on the cosign login", workflow: in("sign", "          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n", "          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n          EXTRA: x\n"), wantIn: "env holds"},
		{
			name:     "the token pasted into the cosign login",
			workflow: in("sign", `run: printf '%s' "$GITHUB_TOKEN" | cosign login ghcr.io --username "$ACTOR" --password-stdin`, `run: printf '%s' "${{ secrets.GITHUB_TOKEN }}" | cosign login ghcr.io --username "$ACTOR" --password-stdin`),
			wantIn:   "holds a ${{ }} expression",
		},
		{name: "the cosign login without the actor from env", workflow: in("sign", `cosign login ghcr.io --username "$ACTOR"`, `cosign login ghcr.io --username "github-actions"`), wantIn: "run is"},

		// The signature: keyless, recursive, by digest, with the digest through
		// env:.
		{name: "the sign step without --recursive", workflow: in("sign", `run: cosign sign --yes --recursive "ghcr.io/timothymamo/eusov@${DIGEST}"`, `run: cosign sign --yes "ghcr.io/timothymamo/eusov@${DIGEST}"`), wantIn: "--recursive"},
		{name: "the sign step by tag", workflow: in("sign", `run: cosign sign --yes --recursive "ghcr.io/timothymamo/eusov@${DIGEST}"`, `run: cosign sign --yes --recursive "ghcr.io/timothymamo/eusov:${DIGEST}"`), wantIn: "@${DIGEST}"},
		{name: "the sign step without the digest env", workflow: in("sign", "        env:\n          DIGEST: ${{ needs.image.outputs.digest }}\n", ""), wantIn: `no "env"`},
		{name: "a shell on the sign step", workflow: in("sign", "        env:\n          DIGEST: ${{ needs.image.outputs.digest }}\n", "        shell: bash\n        env:\n          DIGEST: ${{ needs.image.outputs.digest }}\n"), wantIn: `key "shell"`},
		{name: "GOFLAGS on the sign step", workflow: in("sign", "          DIGEST: ${{ needs.image.outputs.digest }}\n", "          DIGEST: ${{ needs.image.outputs.digest }}\n          GOFLAGS: -tags=sneaky\n"), wantIn: "GOFLAGS"},

		// The attestations: the image and its SBOM by digest, pushed to the
		// registry, and nothing stored in an organisation API.
		{name: "provenance not pushed to the registry", workflow: in("sign", "          push-to-registry: true\n          create-storage-record: false # an organisation API; needs artifact-metadata: write\n", "          push-to-registry: false\n          create-storage-record: false # an organisation API; needs artifact-metadata: write\n"), wantIn: "push-to-registry"},
		{name: "the SBOM attestation not pushed to the registry", workflow: in("sign", "          sbom-path: eusov.spdx.json\n          push-to-registry: true\n", "          sbom-path: eusov.spdx.json\n          push-to-registry: false\n"), wantIn: "push-to-registry"},
		{name: "the provenance attestation stored in the repository", workflow: in("sign", "          create-storage-record: false # an organisation API; needs artifact-metadata: write\n", "          create-storage-record: true # an organisation API; needs artifact-metadata: write\n"), wantIn: "create-storage-record"},
		{name: "the SBOM attestation stored in the repository", workflow: in("sign", "          push-to-registry: true\n          create-storage-record: false\n", "          push-to-registry: true\n          create-storage-record: true\n"), wantIn: "create-storage-record"},
		{name: "a provenance attestation of another subject", workflow: in("sign", "      - name: Attest the build provenance\n        uses: actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6 # v4.2.2\n        with:\n          subject-name: ghcr.io/timothymamo/eusov\n", "      - name: Attest the build provenance\n        uses: actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6 # v4.2.2\n        with:\n          subject-name: ghcr.io/example/eusov\n"), wantIn: "subject-name"},
		{name: "an SBOM attestation of another digest", workflow: in("sign", "          subject-digest: ${{ needs.image.outputs.digest }}\n          sbom-path: eusov.spdx.json\n", "          subject-digest: ${{ github.sha }}\n          sbom-path: eusov.spdx.json\n"), wantIn: "subject-digest"},
		{name: "an SBOM attestation without the SBOM", workflow: in("sign", "          sbom-path: eusov.spdx.json\n", ""), wantIn: `no "sbom-path"`},
		{name: "the provenance attested from a file", workflow: in("sign", "      - name: Attest the build provenance\n        uses: actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6 # v4.2.2\n        with:\n          subject-name: ghcr.io/timothymamo/eusov\n", "      - name: Attest the build provenance\n        uses: actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6 # v4.2.2\n        with:\n          subject-path: dist/eusov\n"), wantIn: "subject-path"},
		{name: "an artifact downloaded under another name", workflow: in("sign", "      - uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2\n        with:\n          name: sbom\n", "      - uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2\n        with:\n          name: eusov-sbom\n"), wantIn: "name"},
		{name: "another action in place of the artifact download", workflow: in("sign", "      - uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2\n", "      - uses: example/attest@0123456789abcdef0123456789abcdef01234567\n"), wantIn: "download-artifact"},

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
