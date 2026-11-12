package check

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/timothymamo/shift2eu/test/internal/yamlread"
)

// The release workflow is read as a closed schema: every key, job and step it
// may hold is listed below, with the value it must have. A pin a Renovate
// update changes (an action's commit, goreleaser's, cosign's and syft's
// versions, the BuildKit image) is matched by form, everything else exactly.
var (
	// actionPin matches the commit an action is pinned to.
	actionPin = `@[0-9a-f]{40}$`

	checkoutAction         = regexp.MustCompile(`^actions/checkout` + actionPin)
	setupGoAction          = regexp.MustCompile(`^actions/setup-go` + actionPin)
	goreleaserAction       = regexp.MustCompile(`^goreleaser/goreleaser-action` + actionPin)
	setupBuildxAction      = regexp.MustCompile(`^docker/setup-buildx-action` + actionPin)
	loginAction            = regexp.MustCompile(`^docker/login-action` + actionPin)
	metadataAction         = regexp.MustCompile(`^docker/metadata-action` + actionPin)
	buildPushAction        = regexp.MustCompile(`^docker/build-push-action` + actionPin)
	sbomAction             = regexp.MustCompile(`^anchore/sbom-action` + actionPin)
	uploadArtifactAction   = regexp.MustCompile(`^actions/upload-artifact` + actionPin)
	cosignInstallerAction  = regexp.MustCompile(`^sigstore/cosign-installer` + actionPin)
	downloadArtifactAction = regexp.MustCompile(`^actions/download-artifact` + actionPin)
	attestAction           = regexp.MustCompile(`^actions/attest` + actionPin)
	provenanceAction       = regexp.MustCompile(`^actions/attest-build-provenance` + actionPin)

	// exactRelease matches one exact release of a tool: goreleaser, cosign,
	// syft.
	exactRelease = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

	// buildkitImage matches the BuildKit image setup-buildx-action runs,
	// pinned by an exact release and the digest of its multi-arch index.
	buildkitImage = regexp.MustCompile(`^image=moby/buildkit:v[0-9]+\.[0-9]+\.[0-9]+@sha256:[0-9a-f]{64}$`)
)

// releaseStep is one step a job of the release workflow must run. The keys
// the step holds are exactly those whose field is set.
type releaseStep struct {
	// what names the step in a message.
	what string
	// name and id are the step's name: and id:, "" for none.
	name, id string
	// uses matches the action the step runs, nil for a run: step.
	uses *regexp.Regexp
	// with is the step's whole with: mapping, each key with what its value
	// must match; nil for none. A literal block value (|) is matched as its
	// lines joined by newlines.
	with map[string]*regexp.Regexp
	// env is the step's whole env: mapping, line by line; nil for none.
	env []string
	// run is the step's command, a literal block's lines joined by newlines;
	// "" for none.
	run string
}

// exactly matches value and nothing else.
func exactly(value string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(value) + `$`)
}

// releaseImage is one container image the release publishes, by three jobs of
// its own: one builds and pushes it, one reads its SBOM, one signs and attests
// it. Each image is held to the same steps; only these values differ.
type releaseImage struct {
	// name is the image's repository in the registry.
	name string
	// target is the Dockerfile stage it is built from, "" for the last one.
	target string
	// imageJob, sbomJob and signJob are the keys of its three jobs, and
	// suffix what their names add to the CLI image's job names.
	imageJob, sbomJob, signJob, suffix string
	// sbomFile is the SPDX document of the image, and sbomArtifact the name
	// of the workflow artifact the SBOM job hands it to the sign job under.
	sbomFile, sbomArtifact string
	// ciVar is the variable ci.yml's image job names the image it built for
	// this one by, so the dry run's container runs mirror each image's own.
	ciVar string
}

// digest is the expression through which the image's SBOM and sign jobs read
// the digest its image job built, to read, sign and attest the image by.
func (img releaseImage) digest() string {
	return "${{ needs." + img.imageJob + ".outputs.digest }}"
}

// The images the release publishes: the CLI, from the Dockerfile's last stage
// (ADR 0013, ADR 0015), and the HTTP API, from its server stage (PRD #103, ADR
// 0019).
var (
	cliImage = releaseImage{
		name: "ghcr.io/timothymamo/shift2eu", imageJob: "image", sbomJob: "sbom", signJob: "sign",
		sbomFile: "shift2eu.spdx.json", sbomArtifact: "sbom", ciVar: "IMAGE",
	}
	serverImage = releaseImage{
		name: "ghcr.io/timothymamo/shift2eu-server", target: "server",
		imageJob: "server-image", sbomJob: "server-sbom", signJob: "server-sign", suffix: " (shift2eu-server)",
		sbomFile: "shift2eu-server.spdx.json", sbomArtifact: "server-sbom", ciVar: "SERVER_IMAGE",
	}
	releaseImages = []releaseImage{cliImage, serverImage}
)

// The expression of the digest an image job builds and gives as its output,
// and the registry every job logs in to.
const (
	builtDigest   = "${{ steps.build.outputs.digest }}"
	registryLogin = "ghcr.io"
)

var (
	// setupGoStep is setup-go with its Actions cache off: the cache would
	// restore GOMODCACHE and GOCACHE, which the go command trusts without
	// verifying them again, from an earlier job.
	setupGoStep = releaseStep{
		what: "actions/setup-go", uses: setupGoAction,
		with: map[string]*regexp.Regexp{"go-version-file": exactly("go.mod"), "cache": exactly("false")},
	}

	// fullCheckoutStep checks out the whole history without keeping the token.
	fullCheckoutStep = releaseStep{
		what: "actions/checkout", uses: checkoutAction,
		with: map[string]*regexp.Regexp{"fetch-depth": exactly("0"), "persist-credentials": exactly("false")},
	}

	// loginStep logs in to GHCR with the job's own token.
	loginStep = releaseStep{
		what: "docker/login-action", uses: loginAction,
		with: map[string]*regexp.Regexp{
			"registry": exactly(registryLogin),
			"username": exactly("${{ github.actor }}"),
			"password": exactly("${{ secrets.GITHUB_TOKEN }}"),
		},
	}

	// testSteps are the test job's steps: the whole history, the guard that
	// the tagged commit is on main (ADR 0015) before anything runs repository
	// code, and the tests, on a runner of their own.
	testSteps = []releaseStep{
		fullCheckoutStep,
		{
			what: "the guard that the tagged commit is on main", name: "The tagged commit is on main",
			run: strings.Join([]string{
				`git rev-parse --verify --quiet refs/remotes/origin/main > /dev/null || {`,
				`  echo "origin/main is missing from the checkout" >&2`,
				`  exit 1`,
				`}`,
				`git merge-base --is-ancestor "$GITHUB_SHA" refs/remotes/origin/main || {`,
				`  echo "$GITHUB_SHA is not on main: nothing is released" >&2`,
				`  exit 1`,
				`}`,
			}, "\n"),
		},
		setupGoStep,
		{what: "make test", name: "Test", run: "make test"},
	}

	// goreleaserSteps are the goreleaser job's steps: no repository code runs
	// before goreleaser, which is asked for exactly release --clean, and the
	// archives and checksums.txt leave the job as a short-lived workflow
	// artifact for the attest-archives job (issue #163).
	goreleaserSteps = []releaseStep{
		fullCheckoutStep,
		setupGoStep,
		{
			what: "goreleaser/goreleaser-action", uses: goreleaserAction,
			with: map[string]*regexp.Regexp{
				"distribution": exactly("goreleaser"),
				"version":      exactRelease,
				"args":         exactly("release --clean"),
			},
			env: []string{"GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}"},
		},
		{
			what: "actions/upload-artifact (the archives)", uses: uploadArtifactAction,
			with: map[string]*regexp.Regexp{
				"name":              exactly(archivesArtifact),
				"path":              exactly("dist/" + strings.Join(attestedArchives, "\ndist/")),
				"if-no-files-found": exactly("error"),
				"retention-days":    exactly("1"),
			},
		},
	}
)

// The release's archives and checksums.txt: the workflow artifact that hands
// them from the goreleaser job to the attest-archives job, and the patterns
// of the files that job attests, which verify-archives downloads from the
// published release and verifies (issue #163).
const archivesArtifact = "archives"

// attestedArchives are the files handed on and attested, each archive named by
// its extension: a bare shift2eu_* in dist/ also matches GoReleaser's build
// directories (dist/shift2eu_linux_amd64_v1/ ...), and would hand on and
// attest the bare binaries beside the archives.
var attestedArchives = []string{
	"shift2eu_*.tar.gz", "shift2eu_*.zip",
	"shift2eu-server_*.tar.gz", "shift2eu-server_*.zip",
	"checksums.txt",
}

// archiveSubjects are the patterns verify-archives downloads and verifies: a
// published release holds only the archives and checksums.txt, so a bare
// pattern names exactly them there.
var archiveSubjects = []string{"shift2eu_*", "shift2eu-server_*", "checksums.txt"}

// attestArchivesSteps are the attest-archives job's steps: GitHub's own
// download of the archives' workflow artifact, the check of the archives
// against checksums.txt, and one build provenance attestation of every
// archive and of checksums.txt itself, by path, so each file is a subject.
var attestArchivesSteps = []releaseStep{
	{
		what: "actions/download-artifact (the archives)", uses: downloadArtifactAction,
		with: map[string]*regexp.Regexp{"name": exactly(archivesArtifact)},
	},
	{what: "the check against checksums.txt", name: "Check the archives against checksums.txt", run: "sha256sum -c checksums.txt"},
	{
		what: "actions/attest-build-provenance (the archives)", name: "Attest the archives", uses: provenanceAction,
		with: map[string]*regexp.Regexp{"subject-path": exactly(strings.Join(attestedArchives, "\n"))},
	},
}

// ghEnv is the env: of a gh step: the tag, and the job's own token for gh.
var ghEnv = []string{tagEnv, "GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}"}

// publishSteps are the publish job's steps: the one command that publishes
// the draft release GoReleaser uploaded to (issue #156). gh is given the
// repository with --repo: the job checks nothing out, so gh has no git
// repository to read it from.
var publishSteps = []releaseStep{{
	what: "the publication of the draft release", name: "Publish the draft release",
	env: ghEnv,
	run: `gh release edit "$TAG" --repo ` + releaseRepo + ` --draft=false`,
}}

// verifyArchivesSteps are the verify-archives job's steps: the published
// release's own archives and checksums.txt, their digests, and the build
// provenance of each, made by the release workflow for this tag and this
// commit on GitHub's runners (docs/guide/verify-releases.md, issue #163).
var verifyArchivesSteps = []releaseStep{
	{
		what: "the download of the published archives", name: "Download the published archives",
		env: ghEnv,
		run: strings.Join([]string{
			`gh release download "$TAG" --repo ` + releaseRepo + ` \`,
			`  --pattern 'shift2eu_*' --pattern 'shift2eu-server_*' --pattern checksums.txt`,
		}, "\n"),
	},
	{
		what: "the archives' verification", name: "Verify the published archives",
		env: ghEnv,
		run: strings.Join([]string{
			strictShell,
			`sha256sum -c checksums.txt`,
			`for subject in ` + strings.Join(archiveSubjects, " ") + `; do`,
			`  gh attestation verify "$subject" --repo ` + releaseRepo + ` \`,
			`    --signer-workflow ` + signerWorkflow + ` \`,
			`    --source-ref "refs/tags/$TAG" --source-digest "$GITHUB_SHA" --deny-self-hosted-runners`,
			`done`,
		}, "\n"),
	},
}

// imageSteps are the image job's steps: the build arguments GoReleaser
// stamps and whether the tag is the highest release tag on main, read from git
// with the tag passed through env:, refusing a checkout without origin/main
// because the run shell has no pipefail and the pipeline would otherwise
// leave highest empty and latest false; a build of two platforms from an empty
// BuildKit, pushed, without BuildKit's own provenance or SBOM, tagged
// latest by the stamp's decision alone. Nothing signs or attests here.
func imageSteps(img releaseImage) []releaseStep {
	return []releaseStep{
		fullCheckoutStep,
		{
			what: "the stamp of the build arguments", name: "Build arguments, as GoReleaser stamps them", id: "stamp",
			env: []string{"TAG: ${{ github.ref_name }}"},
			run: strings.Join([]string{
				`git rev-parse --verify --quiet refs/remotes/origin/main > /dev/null || {`,
				`  echo "origin/main is missing from the checkout" >&2`,
				`  exit 1`,
				`}`,
				`highest="$(git tag -l 'v*.*.*' --merged refs/remotes/origin/main | sed -nE '/^v[0-9]+\.[0-9]+\.[0-9]+$/p' | sort -V | tail -n 1)"`,
				`latest=false`,
				`if [ "$TAG" = "$highest" ]; then latest=true; fi`,
				`{`,
				`  echo "version=${TAG#v}"`,
				`  echo "commit=$(git rev-parse --short HEAD)"`,
				`  echo "date=$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ)"`,
				`  echo "latest=$latest"`,
				`} >> "$GITHUB_OUTPUT"`,
			}, "\n"),
		},
		{
			what: "docker/setup-buildx-action", uses: setupBuildxAction,
			with: map[string]*regexp.Regexp{"driver-opts": buildkitImage},
		},
		loginStep,
		{
			what: "docker/metadata-action", name: "Image tags", id: "tags", uses: metadataAction,
			with: map[string]*regexp.Regexp{
				"images": exactly(img.name),
				"flavor": exactly("latest=${{ steps.stamp.outputs.latest }}"),
				"tags": exactly(strings.Join([]string{
					"type=semver,pattern={{raw}}",
					"type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}",
				}, "\n")),
			},
		},
		{
			what: "docker/build-push-action", name: "Build and push linux/amd64 and linux/arm64", id: "build", uses: buildPushAction,
			with: buildWith(img),
		},
	}
}

// buildWith is the with: of an image's build: the release's build arguments
// and settings, and, for an image of a stage other than the last, the stage
// as its target.
func buildWith(img releaseImage) map[string]*regexp.Regexp {
	with := map[string]*regexp.Regexp{
		"context":     exactly("."),
		"platforms":   exactly("linux/amd64,linux/arm64"),
		"push":        exactly("true"),
		"tags":        exactly("${{ steps.tags.outputs.tags }}"),
		"annotations": exactly("index:org.opencontainers.image.source=https://github.com/timothymamo/shift2eu"),
		"provenance":  exactly("false"),
		"sbom":        exactly("false"),
		"build-args": exactly(strings.Join([]string{
			"VERSION=${{ steps.stamp.outputs.version }}",
			"COMMIT=${{ steps.stamp.outputs.commit }}",
			"DATE=${{ steps.stamp.outputs.date }}",
		}, "\n")),
	}

	if img.target != "" {
		with["target"] = exactly(img.target)
	}

	return with
}

// sbomSteps are the sbom job's steps: a read-only login, syft on the
// pushed index by digest (an exact syft, uploading nothing itself), and
// the SPDX document handed to the sign job as a short-lived workflow
// artifact.
func sbomSteps(img releaseImage) []releaseStep {
	return []releaseStep{
		loginStep,
		{
			what: "anchore/sbom-action", uses: sbomAction,
			with: map[string]*regexp.Regexp{
				"image":                 exactly("registry:" + img.name + "@" + img.digest()),
				"format":                exactly("spdx-json"),
				"output-file":           exactly(img.sbomFile),
				"upload-artifact":       exactly("false"),
				"upload-release-assets": exactly("false"),
				"syft-version":          exactRelease,
			},
		},
		{
			what: "actions/upload-artifact", uses: uploadArtifactAction,
			with: map[string]*regexp.Regexp{
				"name":              exactly(img.sbomArtifact),
				"path":              exactly(img.sbomFile),
				"if-no-files-found": exactly("error"),
				"retention-days":    exactly("1"),
			},
		},
	}
}

// signSteps are the sign job's steps, sigstore's and GitHub's code alone:
// cosign by exact release; a cosign login fed the actor and the token
// through env:; a keyless, recursive signature by digest; the provenance
// attestation; the SBOM artifact; and its attestation, both by digest,
// pushed to the registry and stored in no organisation API.
func signSteps(img releaseImage) []releaseStep {
	return []releaseStep{
		cosignInstallerStep,
		cosignLoginStep,
		{
			what: "the signature", name: "Sign the image",
			env: []string{"DIGEST: " + img.digest()},
			run: `cosign sign --yes --recursive "` + img.name + `@${DIGEST}"`,
		},
		{
			what: "actions/attest (the build provenance)", name: "Attest the build provenance", uses: attestAction,
			with: map[string]*regexp.Regexp{
				"subject-name":          exactly(img.name),
				"subject-digest":        exactly(img.digest()),
				"push-to-registry":      exactly("true"),
				"create-storage-record": exactly("false"),
			},
		},
		{
			what: "actions/download-artifact", uses: downloadArtifactAction,
			with: map[string]*regexp.Regexp{"name": exactly(img.sbomArtifact)},
		},
		{
			what: "actions/attest (the SBOM)", name: "Attest the SBOM", uses: attestAction,
			with: map[string]*regexp.Regexp{
				"subject-name":          exactly(img.name),
				"subject-digest":        exactly(img.digest()),
				"sbom-path":             exactly(img.sbomFile),
				"push-to-registry":      exactly("true"),
				"create-storage-record": exactly("false"),
			},
		},
	}
}

var (
	// cosignInstallerStep installs cosign by exact release.
	cosignInstallerStep = releaseStep{
		what: "sigstore/cosign-installer", uses: cosignInstallerAction,
		with: map[string]*regexp.Regexp{"cosign-release": exactRelease},
	}

	// cosignLoginStep logs cosign, and through its credential docker and
	// buildx, in to GHCR with the job's own token, fed through env:.
	cosignLoginStep = releaseStep{
		what: "the cosign login", name: "Log in to ghcr.io",
		env: []string{"ACTOR: ${{ github.actor }}", "GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}"},
		run: `printf '%s' "$GITHUB_TOKEN" | cosign login ` + registryLogin + ` --username "$ACTOR" --password-stdin`,
	}
)

// The release identity every published image is verified against: the
// release workflow, run on a push of exactly the tag being verified.
const (
	releaseRepo     = "timothymamo/shift2eu"
	signerWorkflow  = releaseRepo + "/.github/workflows/release.yml"
	spdxPredicate   = "https://spdx.dev/Document/v2.3"
	tagEnv          = "TAG: ${{ github.ref_name }}"
	verifyJob       = "verify"
	verifyJobName   = "Verify the published images"
	tagRuleStepName = "The tags follow the release rule"
)

// verifyImageStep is the verify job's step for img, the checks of
// docs/guide/verify-releases.md on the digest its image job built: the tag
// must resolve to that digest, and that image must carry the signature this
// workflow made for exactly this tag and this commit, the build provenance and
// the SBOM, attested from this workflow for this tag and commit on GitHub's
// runners, and run with --rm --read-only --network none and then smoke.
func verifyImageStep(img releaseImage, what, name string, smoke ...string) releaseStep {
	attest := []string{
		`gh attestation verify "oci://$IMAGE" --repo ` + releaseRepo + ` \`,
		`  --signer-workflow ` + signerWorkflow + ` \`,
		`  --source-ref "refs/tags/$TAG" --source-digest "$GITHUB_SHA" --deny-self-hosted-runners`,
	}

	sbom := slices.Clone(attest)
	sbom[len(sbom)-1] += ` \`
	sbom = append(sbom, `  --predicate-type `+spdxPredicate)

	return releaseStep{
		what: what, name: name,
		env: []string{tagEnv, "GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}", "BUILT: " + img.digest()},
		run: strings.Join(slices.Concat(
			[]string{
				strictShell,
				`DIGEST="$(docker buildx imagetools inspect "` + img.name + `:$TAG" --format '{{.Manifest.Digest}}')"`,
				`if [ "$DIGEST" != "$BUILT" ]; then`,
				`  echo "` + img.name + `:$TAG resolves to $DIGEST, not the digest this run built, $BUILT" >&2`,
				`  exit 1`,
				`fi`,
				`IMAGE="` + img.name + `@$DIGEST"`,
				`cosign verify "$IMAGE" \`,
				`  --certificate-identity "https://github.com/` + signerWorkflow + `@refs/tags/$TAG" \`,
				`  --certificate-oidc-issuer https://token.actions.githubusercontent.com \`,
				`  --certificate-github-workflow-trigger push \`,
				`  --certificate-github-workflow-sha "$GITHUB_SHA"`,
			},
			attest,
			sbom,
			smoke,
		), "\n"),
	}
}

// strictShell is the first line of every verify script: the run shell is
// bash -e without pipefail or nounset, so an error inside a pipeline or an
// unset variable would otherwise pass unseen.
const strictShell = "set -euo pipefail"

// verifySteps are the verify job's steps: cosign by exact release, a cosign
// login, the checks of each image this run built, by digest, and the tag
// rule, whose highest release tag is read by ancestry from main in a bare
// repository of the job's own. No checkout: the job runs no repository code.
var verifySteps = []releaseStep{
	cosignInstallerStep,
	cosignLoginStep,
	verifyImageStep(cliImage, "the CLI image's verification", "Verify the CLI image",
		`docker run --rm --read-only --network none "$IMAGE" version | tee version.txt`,
		`grep -qF "shift2eu ${TAG#v} (" version.txt`,
	),
	// The server has no version subcommand: --help exiting 0 is its smoke test.
	verifyImageStep(serverImage, "the HTTP API's image verification", "Verify the HTTP API's image",
		`docker run --rm --read-only --network none "$IMAGE" --help > /dev/null`,
	),
	{
		what: "the tag rule", name: tagRuleStepName,
		env: []string{tagEnv, "CLI_BUILT: " + cliImage.digest(), "SERVER_BUILT: " + serverImage.digest()},
		run: strings.Join([]string{
			strictShell,
			`version="${TAG#v}"`,
			`minor="${version%%-*}"`,
			`minor="${minor%.*}"`,
			`git init -q tags`,
			`git -C tags fetch -q --filter=tree:0 --no-tags https://github.com/` + releaseRepo + `.git '+refs/heads/main:refs/remotes/origin/main' '+refs/tags/v*:refs/tags/v*'`,
			`highest="$(git -C tags tag -l 'v*.*.*' --merged refs/remotes/origin/main | sed -nE '/^v[0-9]+\.[0-9]+\.[0-9]+$/p' | sort -V | tail -n 1)"`,
			`if [ -z "$highest" ]; then`,
			`  echo "no release tag is merged into main, so X.Y and latest cannot be checked" >&2`,
			`  exit 1`,
			`fi`,
			`# digest_or_absent prints the digest its tag resolves to, or nothing`,
			`# when the registry says the tag does not exist. The digest is what`,
			`# the inspect writes on stdout, in stderr kept apart, so a docker`,
			`# warning is never read as one, and what succeeds must be exactly one`,
			`# sha256 line. Absence is exit status 1 with the fixed, case-sensitive`,
			`# "$1: not found" in stderr, which names this very reference, and`,
			`# nothing else is absence: any other answer, a 5xx, a rate limit, a`,
			`# failed login, a missing docker or a broken credential helper, is`,
			`# printed and fails the step, whose caller assigns the digest in a`,
			`# command of its own so set -e sees the failure.`,
			`digest_or_absent() {`,
			`  err="$(mktemp)"`,
			`  if out="$(docker buildx imagetools inspect "$1" --format '{{.Manifest.Digest}}' 2> "$err")"; then`,
			`    if [[ "$out" =~ ^sha256:[0-9a-f]{64}$ ]]; then`,
			`      printf '%s\n' "$out"`,
			`    else`,
			`      echo "$1 answered '$out' rather than one sha256 digest" >&2`,
			`      rm -f "$err"`,
			`      return 1`,
			`    fi`,
			`  else`,
			`    rc=$?`,
			`    if [ "$rc" -eq 1 ] && grep -qF -e "$1: not found" "$err"; then`,
			`      :`,
			`    else`,
			`      echo "$1 could not be read, docker exited $rc:" >&2`,
			`      cat "$err" >&2`,
			`      rm -f "$err"`,
			`      return 1`,
			`    fi`,
			`  fi`,
			`  rm -f "$err"`,
			`}`,
			`check() {`,
			`  image="$1"`,
			`  digest="$2"`,
			`  minor_digest="$(digest_or_absent "$image:$minor")"`,
			`  case "$TAG" in`,
			`    *-*)`,
			`      if [ "$minor_digest" = "$digest" ]; then`,
			`        echo "$image:$minor is $TAG, a pre-release, and a pre-release moves neither X.Y nor latest" >&2`,
			`        exit 1`,
			`      fi`,
			`      latest_digest="$(digest_or_absent "$image:latest")"`,
			`      if [ "$latest_digest" = "$digest" ]; then`,
			`        echo "$image:latest is $TAG, a pre-release, and a pre-release moves neither X.Y nor latest" >&2`,
			`        exit 1`,
			`      fi`,
			`      ;;`,
			`    *)`,
			`      if [ "$minor_digest" != "$digest" ]; then`,
			`        echo "$image:$minor is not $TAG" >&2`,
			`        exit 1`,
			`      fi`,
			`      if [ "$highest" = "$TAG" ]; then`,
			`        latest_digest="$(digest_or_absent "$image:latest")"`,
			`        if [ "$latest_digest" != "$digest" ]; then`,
			`          echo "$image:latest is not $TAG, the highest release tag" >&2`,
			`          exit 1`,
			`        fi`,
			`      else`,
			`        highest_digest="$(digest_or_absent "$image:$highest")"`,
			`        if [ -z "$highest_digest" ]; then`,
			`          echo "::notice::$image:$highest is not published yet, so this run does not check $image:latest"`,
			`        else`,
			`          latest_digest="$(digest_or_absent "$image:latest")"`,
			`          if [ "$latest_digest" = "$digest" ]; then`,
			`            echo "$image:latest is $TAG, which is not the highest release tag, $highest" >&2`,
			`            exit 1`,
			`          fi`,
			`        fi`,
			`      fi`,
			`      ;;`,
			`  esac`,
			`}`,
			`check ` + cliImage.name + ` "$CLI_BUILT"`,
			`check ` + serverImage.name + ` "$SERVER_BUILT"`,
		}, "\n"),
	},
}

// releaseJob is one job of the release workflow.
type releaseJob struct {
	key, name string
	// needs are the jobs it needs, in the order written; nil for none. One
	// job may be written as a single name or a list of one.
	needs []string
	// permissions is the job's whole permissions: mapping.
	permissions map[string]string
	// outputs is the job's whole outputs: mapping; nil for none.
	outputs map[string]string
	steps   []releaseStep
}

// releaseJobs are the release workflow's jobs, in the order a message lists
// them: test, goreleaser, attest-archives and publish, then three per image,
// then verify and verify-archives. The write permissions stay split: the
// goreleaser job writes the repository's contents to upload to a draft
// release, and publish alone publishes it, holding nothing else; an image job
// pushes and nothing more; an sbom job, which runs syft, can only read the
// registry; a sign job and attest-archives alone hold an OIDC token and write
// attestations, attest-archives with read-only contents; and the verify jobs
// read and write nothing.
var releaseJobs = func() []releaseJob {
	jobs := []releaseJob{
		{key: "test", name: "Test", permissions: map[string]string{"contents": "read"}, steps: testSteps},
		{
			key: "goreleaser", name: "GoReleaser", needs: []string{"test"},
			permissions: map[string]string{"contents": "write"}, steps: goreleaserSteps,
		},
		{
			key: "attest-archives", name: "Attest the archives", needs: []string{"goreleaser"},
			permissions: map[string]string{"id-token": "write", "attestations": "write", "contents": "read"},
			steps:       attestArchivesSteps,
		},
		{
			key: "publish", name: "Publish the release", needs: []string{"test", "goreleaser", "attest-archives"},
			permissions: map[string]string{"contents": "write"}, steps: publishSteps,
		},
	}

	for _, img := range releaseImages {
		jobs = append(jobs,
			releaseJob{
				key: img.imageJob, name: "Container image" + img.suffix, needs: []string{"test", "goreleaser"},
				permissions: map[string]string{"contents": "read", "packages": "write"},
				outputs:     map[string]string{"digest": builtDigest},
				steps:       imageSteps(img),
			},
			releaseJob{
				key: img.sbomJob, name: "SBOM" + img.suffix, needs: []string{img.imageJob},
				permissions: map[string]string{"packages": "read"}, steps: sbomSteps(img),
			},
			releaseJob{
				key: img.signJob, name: "Sign and attest" + img.suffix, needs: []string{img.imageJob, img.sbomJob},
				permissions: map[string]string{"packages": "write", "id-token": "write", "attestations": "write"},
				steps:       signSteps(img),
			},
		)
	}

	return append(jobs,
		releaseJob{
			key: verifyJob, name: verifyJobName,
			needs:       []string{cliImage.imageJob, cliImage.signJob, serverImage.imageJob, serverImage.signJob},
			permissions: map[string]string{"packages": "read", "attestations": "read"},
			steps:       verifySteps,
		},
		releaseJob{
			key: "verify-archives", name: "Verify the published archives", needs: []string{"publish"},
			permissions: map[string]string{"contents": "read", "attestations": "read"},
			steps:       verifyArchivesSteps,
		},
	)
}()

// ReleaseWorkflow returns a message for everything in the release workflow
// document src that could make goreleaser build, or the publishing jobs
// publish, something other than what the check lists. It reads the document
// as a closed schema:
//
//   - the top level holds exactly name, on (a push of v*.*.* tags),
//     permissions ({}), concurrency (exactly group: release and
//     cancel-in-progress: false, so one release runs at a time across every
//     tag and a run in progress is never cancelled half published; a waiting
//     run may be replaced by a newer tag's and must then be re-run by hand,
//     ADR 0015) and jobs, so no workflow-level env: or defaults:;
//   - jobs holds exactly test, goreleaser, attest-archives, publish, an
//     image, an sbom and a sign job for each image the release publishes: image, sbom and sign for the
//     CLI's, ghcr.io/timothymamo/shift2eu; server-image, server-sbom and
//     server-sign for the HTTP API's, ghcr.io/timothymamo/shift2eu-server
//     (PRD #103), whose job names end " (shift2eu-server)"; verify, the
//     independent check of the images the release published (issue #99); and
//     verify-archives, that of its archives (issue #163). Each holds
//     exactly name, runs-on (ubuntu-latest), permissions and steps, and needs
//     for all but test (goreleaser needs test; attest-archives goreleaser;
//     publish [test, goreleaser, attest-archives]; an image job [test,
//     goreleaser]; an sbom job its image job; a sign job [its image job, its
//     sbom job], in that order; verify [image, sign, server-image,
//     server-sign]; verify-archives publish), and an image job
//     an outputs: of exactly its build's digest, so no job-level env:,
//     container:, if: or other key;
//   - the permissions are exactly contents: read for test; contents: write
//     for goreleaser and for publish; id-token: write, attestations: write
//     and contents: read for attest-archives; contents: read and packages:
//     write for an image job; packages: read for an sbom job; packages:
//     write, id-token: write and attestations: write for a sign job;
//     packages: read and attestations: read for verify, and contents: read
//     and attestations: read for verify-archives, so no OIDC token and
//     nothing either can write;
//   - the test job's steps are exactly actions/checkout of the whole history,
//     the guard that the tagged commit is on main (an exact run: script),
//     actions/setup-go and make test; the goreleaser job's exactly
//     actions/checkout, actions/setup-go, goreleaser/goreleaser-action and
//     actions/upload-artifact of the archives, each named by its extension,
//     and checksums.txt (archives, kept one day), setup-go with cache: false in both jobs, goreleaser
//     asked for exactly release --clean at an exact version, with an env: of
//     exactly GITHUB_TOKEN from secrets.GITHUB_TOKEN;
//   - the attest-archives job's steps are exactly actions/download-artifact
//     of that artifact, sha256sum -c checksums.txt and
//     actions/attest-build-provenance with a subject-path of every archive,
//     by its extension, and checksums.txt; the publish job's exactly gh
//     release edit "$TAG" --repo timothymamo/shift2eu --draft=false, which publishes the draft release GoReleaser uploaded
//     to (.goreleaser.yaml's release.draft, check.GitHubRelease), with the tag
//     and token through env:, the one step of the workflow that publishes it
//     (issue #156);
//   - an image job's steps are exactly actions/checkout, the stamp of the
//     build arguments and of the latest decision (a run: script, with the
//     tag through env:, counting only the release tags merged into
//     origin/main, and refusing a checkout without origin/main, since the
//     run shell has no pipefail to fail the pipeline that counts them),
//     docker/setup-buildx-action (BuildKit by exact release and index
//     digest), docker/login-action, docker/metadata-action (its image's name,
//     latest from the stamp's output alone) and docker/build-push-action
//     (linux/amd64 and linux/arm64, pushed, no cache, no BuildKit provenance
//     or SBOM; the HTTP API's with target: server, the CLI's with no target,
//     so the Dockerfile's last stage);
//   - an sbom job's steps are exactly docker/login-action, anchore/sbom-action
//     (its pushed index by its image job's digest, an exact syft, uploading
//     nothing) and actions/upload-artifact of its SPDX document
//     (shift2eu.spdx.json as sbom, shift2eu-server.spdx.json as server-sbom);
//   - a sign job's steps are exactly sigstore/cosign-installer (an exact
//     release), a cosign login with the actor and token through env:, cosign
//     sign --recursive of its image by its image job's digest,
//     actions/attest of the build provenance, actions/download-artifact of
//     its SPDX document and actions/attest of the SBOM, both by that digest,
//     pushed to the registry;
//   - the verify job's steps are exactly sigstore/cosign-installer, the cosign
//     login, one step per image that, under set -euo pipefail, resolves its
//     tag to a digest, fails unless it is the digest its image job built
//     (BUILT through env:), and, by that digest, runs cosign verify with the
//     release identity and this commit (--certificate-github-workflow-sha),
//     gh attestation verify of the provenance and of the SBOM, both bound to
//     this commit (--source-digest), and the image with --rm --read-only
//     --network none (version, compared to the tag, for the CLI; --help for
//     the server), and the tag rule, whose highest release tag is the release
//     tags merged into main, read by ancestry from a treeless fetch into a
//     repository of the job's own, with no checkout;
//   - the verify-archives job's steps are exactly gh release download, with
//     --repo, of the published release's archives and checksums.txt, and, under set -euo
//     pipefail, sha256sum -c checksums.txt and gh attestation verify of every
//     archive and of checksums.txt with the release identity, bound to this
//     tag and commit, on GitHub's runners only;
//   - every step holds exactly the keys listed (name, id, uses, with, env,
//     run), in that order of steps, each action pinned by commit, each with:
//     key and value, env: line and run: command as listed. A run: command is
//     one line or a literal block (|), compared line by line, and holds no
//     ${{ }} expression: what a step takes from the workflow comes in through
//     env:.
//
// The goreleaser job therefore runs no repository code before goreleaser, and
// make test, which does, runs on another runner: neither the go env file nor
// GITHUB_ENV or GITHUB_PATH carries over from it. The one other channel
// between jobs, setup-go's Actions cache of GOMODCACHE and GOCACHE (shared by
// every job with the same runner OS, Go version and go.sum, CI on main
// included), is closed by cache: false: the go command trusts what GOCACHE
// holds and does not verify extracted modules again. The variables of the go
// command can come only from the runner image or from inside a pinned action.
//
// As a backstop, no line, comments included, may name github.env or
// github.path in any case, nor GITHUB_ENV, GITHUB_PATH or a variable of the
// go command or of cgo (a word starting GO or CGO in upper case: GOFLAGS,
// GOEXPERIMENT, GOAMD64, CGO_ENABLED, ...).
func ReleaseWorkflow(src []byte) []string {
	var messages []string

	raw := strings.Split(string(src), "\n")
	for i := range raw {
		raw[i] = strings.TrimSuffix(raw[i], "\r")
		messages = append(messages, forbiddenNames(strings.TrimSpace(raw[i]))...)
	}

	lines, err := yamlread.SplitLines(src)
	if err != nil {
		return append(messages, fmt.Sprintf("read the release workflow: %v", err))
	}

	return append(messages, releaseSchema(raw, lines)...)
}

// releaseSchema returns a message for every way lines, the content lines of
// the release workflow whose every line is raw, departs from the schema
// ReleaseWorkflow describes.
func releaseSchema(raw []string, lines []yamlread.Line) []string {
	r := &schemaReader{raw: raw}

	if len(lines) == 0 {
		return []string{"the release workflow is empty, so this check cannot tell what goreleaser is asked to do"}
	}

	top := r.exactKeys(lines, "the release workflow", "name", "on", "permissions", "concurrency", "jobs")
	if top == nil {
		return r.messages
	}

	r.scalar(top["name"], "the release workflow's name", exactly("Release"))
	r.rawValue(top["permissions"], "the release workflow's permissions", "{}")

	if c := r.exactKeys(r.block(top["concurrency"], "concurrency"), "concurrency", "group", "cancel-in-progress"); c != nil {
		r.scalar(c["group"], "concurrency: group", exactly("release"))
		r.scalar(c["cancel-in-progress"], "concurrency: cancel-in-progress", exactly("false"))
	}

	if on := r.exactKeys(r.block(top["on"], "on"), "on", "push"); on != nil {
		if push := r.exactKeys(r.block(on["push"], "on: push"), "on: push", "tags"); push != nil {
			r.list(push["tags"], "on: push: tags", []string{"v*.*.*"})
		}
	}

	keys := make([]string, 0, len(releaseJobs))
	for _, j := range releaseJobs {
		keys = append(keys, j.key)
	}

	jobs := r.exactKeys(r.block(top["jobs"], "jobs"), "jobs", keys...)
	if jobs == nil {
		return r.messages
	}

	for _, j := range releaseJobs {
		r.job(jobs[j.key], "jobs: "+j.key, j)
	}

	return r.messages
}

// schemaReader collects the messages of a schema check.
type schemaReader struct {
	// raw are the document's lines, line ends removed, from which a literal
	// block is read.
	raw      []string
	messages []string
}

func (r *schemaReader) fail(format string, args ...any) {
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

// job checks one job against want: its name, needs, runner, permissions,
// outputs and steps.
func (r *schemaReader) job(e yamlread.Entry, path string, want releaseJob) {
	keys := []string{"name", "runs-on", "permissions", "steps"}
	if want.needs != nil {
		keys = append(keys, "needs")
	}

	if want.outputs != nil {
		keys = append(keys, "outputs")
	}

	job := r.exactKeys(r.block(e, path), path, keys...)
	if job == nil {
		return
	}

	r.scalar(job["name"], path+": name", exactly(want.name))
	r.scalar(job["runs-on"], path+": runs-on", exactly("ubuntu-latest"))

	if want.needs != nil {
		r.needs(job["needs"], path+": needs", want.needs)
	}

	r.exactMapping(job["permissions"], path+": permissions", want.permissions)

	if want.outputs != nil {
		r.exactMapping(job["outputs"], path+": outputs", want.outputs)
	}

	stepsPath := path + ": steps"

	block := r.block(job["steps"], stepsPath)
	if block == nil {
		return
	}

	items, err := yamlread.Sequence(block)
	if err != nil {
		r.fail("%s: %v", stepsPath, err)

		return
	}

	if len(items) != len(want.steps) {
		whats := make([]string, 0, len(want.steps))
		for _, s := range want.steps {
			whats = append(whats, s.what)
		}

		r.fail("%s holds %d steps, want exactly %d: %s, so that nothing the check never lists runs in this job",
			stepsPath, len(items), len(want.steps), strings.Join(whats, ", "))

		return
	}

	for i, it := range items {
		r.step(it, fmt.Sprintf("%s: step %d", stepsPath, i+1), want.steps[i])
	}
}

// needs checks that e's value is exactly the jobs want, in that order: a flow
// or block list, or one job name on the key's own line.
func (r *schemaReader) needs(e yamlread.Entry, path string, want []string) {
	var (
		got []string
		err error
	)

	if e.Value == "" || strings.HasPrefix(e.Value, "[") {
		got, err = yamlread.List(e)
	} else {
		var one string

		one, err = yamlread.Single(e)
		got = []string{one}
	}

	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	if !slices.Equal(got, want) {
		r.fail("line %d: %s is %q, want exactly %q", e.Num, path, got, want)
	}
}

// exactMapping checks that e's value is a block mapping holding exactly the
// keys of want, each with exactly its value.
func (r *schemaReader) exactMapping(e yamlread.Entry, path string, want map[string]string) {
	keys := sortedKeys(want)

	if e.Value == "" && len(e.Block) == 0 {
		pairs := make([]string, 0, len(keys))
		for _, key := range keys {
			pairs = append(pairs, key+": "+want[key])
		}

		r.fail("line %d: %s is empty, want exactly %s", e.Num, path, strings.Join(pairs, ", "))

		return
	}

	if got := r.exactKeys(r.block(e, path), path, keys...); got != nil {
		for _, key := range keys {
			r.scalar(got[key], path+": "+key, exactly(want[key]))
		}
	}
}

// step checks one step against want.
func (r *schemaReader) step(it yamlread.Item, path string, want releaseStep) {
	path = fmt.Sprintf("%s (line %d), want %s", path, it.Num, want.what)

	var keys []string

	for _, k := range []struct {
		key string
		set bool
	}{
		{"name", want.name != ""},
		{"id", want.id != ""},
		{"uses", want.uses != nil},
		{"with", want.with != nil},
		{"env", want.env != nil},
		{"run", want.run != ""},
	} {
		if k.set {
			keys = append(keys, k.key)
		}
	}

	step := r.exactKeys(it.Lines, path, keys...)
	if step == nil {
		return
	}

	if want.name != "" {
		r.scalar(step["name"], path+": name", exactly(want.name))
	}

	if want.id != "" {
		r.scalar(step["id"], path+": id", exactly(want.id))
	}

	if want.uses != nil {
		r.scalar(step["uses"], path+": uses", want.uses)
	}

	if want.with != nil {
		withKeys := sortedKeys(want.with)
		if with := r.exactKeys(r.block(step["with"], path+": with"), path+": with", withKeys...); with != nil {
			for _, key := range withKeys {
				r.scalar(with[key], path+": with: "+key, want.with[key])
			}
		}
	}

	if want.env != nil {
		r.env(step["env"], path+": env", want.env)
	}

	if want.run != "" {
		r.run(step["run"], path+": run", want.run)
	}
}

// env checks that e's value is exactly the env: mapping want, line by line:
// its keys are upper case, which the mapping reader refuses.
func (r *schemaReader) env(e yamlread.Entry, path string, want []string) {
	env := r.block(e, path)
	if env == nil {
		return
	}

	got := make([]string, 0, len(env))
	for _, l := range env {
		got = append(got, l.Text)
	}

	if !slices.Equal(got, want) {
		r.fail("%s holds %q, want exactly %q, so the step could run with an environment the check never lists", path, got, want)
	}
}

// run checks that e's value is exactly the command want, and refuses a ${{ }}
// expression in it: the runner pastes the expression's value into the script
// before the shell reads it, so a tag or a branch name could run as a command.
func (r *schemaReader) run(e yamlread.Entry, path, want string) {
	value, err := r.value(e)
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	if strings.Contains(value, "${{") {
		r.fail("line %d: %s holds a ${{ }} expression, whose value the runner pastes into the script as code; pass it through env: instead", e.Num, path)
	}

	if value != want {
		r.fail("line %d: %s is %q, want exactly %q", e.Num, path, value, want)
	}
}

// sortedKeys returns the keys of m in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}

// exactKeys reads lines as a block mapping holding exactly want, and returns
// its entries by key; nil when it cannot, with the reasons in the messages.
func (r *schemaReader) exactKeys(lines []yamlread.Line, path string, want ...string) map[string]yamlread.Entry {
	// A nil block was reported by block already.
	if lines == nil {
		return nil
	}

	entries, err := yamlread.Mapping(lines)
	if err != nil {
		r.fail("%s: %v", path, err)

		return nil
	}

	got := make(map[string]yamlread.Entry, len(entries))
	ok := true

	for _, e := range entries {
		got[e.Key] = e

		if !slices.Contains(want, e.Key) {
			r.fail("%s holds the key %q (line %d), want only %s", path, e.Key, e.Num, strings.Join(want, ", "))

			ok = false
		}
	}

	for _, key := range want {
		if _, found := got[key]; !found {
			r.fail("%s holds no %q, want exactly %s", path, key, strings.Join(want, ", "))

			ok = false
		}
	}

	if !ok {
		return nil
	}

	return got
}

// block returns the lines of e's value, a block below its key; nil when it is
// not one, with the reason in the messages.
func (r *schemaReader) block(e yamlread.Entry, path string) []yamlread.Line {
	lines, err := yamlread.BlockValue(e, path)
	if err != nil {
		r.fail("%v", err)

		return nil
	}

	return lines
}

// scalar checks that e's value, one scalar or a literal block, is one that
// want matches.
func (r *schemaReader) scalar(e yamlread.Entry, path string, want *regexp.Regexp) {
	value, err := r.value(e)
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	if !want.MatchString(value) {
		r.fail("line %d: %s is %q, want one matching %s", e.Num, path, value, want)
	}
}

// value reads e's value: a literal block (|) as its lines joined by newlines,
// anything else as one scalar on the key's own line.
func (r *schemaReader) value(e yamlread.Entry) (string, error) {
	if e.Value == "|" {
		return yamlread.LiteralBlock(r.raw, e)
	}

	return yamlread.Single(e)
}

// rawValue checks that e's value is exactly want, as written on its line.
func (r *schemaReader) rawValue(e yamlread.Entry, path, want string) {
	if e.Value != want || len(e.Block) != 0 {
		r.fail("line %d: %s is %q, want exactly %q", e.Num, path, e.Value, want)
	}
}

// list checks that e's value is exactly the list want.
func (r *schemaReader) list(e yamlread.Entry, path string, want []string) {
	got, err := yamlread.List(e)
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	if !slices.Equal(got, want) {
		r.fail("line %d: %s is %q, want exactly %q", e.Num, path, got, want)
	}
}

// forbiddenNames returns a message for every word of line that names a
// variable of the go command or of cgo, or GITHUB_ENV or GITHUB_PATH, and for
// github.env or github.path in any case.
func forbiddenNames(line string) []string {
	var messages []string

	lower := strings.ToLower(line)

	for _, token := range []string{"github.env", "github.path"} {
		if strings.Contains(lower, token) {
			messages = append(messages, fmt.Sprintf("the release workflow's line %q names %s, through which a step sets the environment or the PATH of the steps after it", line, token))
		}
	}

	words := strings.FieldsFunc(line, func(r rune) bool {
		return r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})

	for _, word := range words {
		switch {
		case word == "GITHUB_ENV" || word == "GITHUB_PATH":
			messages = append(messages, fmt.Sprintf("the release workflow's line %q names %s, through which a step sets the environment of the steps after it, goreleaser's included", line, word))
		case (strings.HasPrefix(word, "GO") || strings.HasPrefix(word, "CGO")) && strings.ToUpper(word) == word:
			messages = append(messages, fmt.Sprintf("the release workflow's line %q names %s, a variable of the go command or of cgo, which would change what goreleaser builds and which the check never lists with", line, word))
		}
	}

	return messages
}

// strayReleaseFiles are the names at the repository root, slash-separated,
// that change what goreleaser builds without the check reading them, each
// with the reason.
var strayReleaseFiles = []struct {
	name   string
	reason string
}{
	{name: ".config/goreleaser.yml", reason: "it is one of the other file names goreleaser looks for, so the release could build from a document the check never reads"},
	{name: ".config/goreleaser.yaml", reason: "it is one of the other file names goreleaser looks for, so the release could build from a document the check never reads"},
	{name: ".goreleaser.yml", reason: "it is one of the other file names goreleaser looks for, so the release could build from a document the check never reads"},
	{name: "goreleaser.yml", reason: "it is one of the other file names goreleaser looks for, so the release could build from a document the check never reads"},
	{name: "goreleaser.yaml", reason: "it is one of the other file names goreleaser looks for, so the release could build from a document the check never reads"},
	{name: "go.work", reason: "the go command goreleaser runs reads it and the check lists with GOWORK=off, so the release could build modules the check never lists"},
	{name: "go.work.sum", reason: "it comes with a go.work, which the go command goreleaser runs reads and the check lists with GOWORK=off"},
	{name: "vendor", reason: "the go command goreleaser runs builds from it and the check lists with -mod=readonly, so the release could build sources the check never lists"},
}

// StrayReleaseFiles returns a message for every file under root that git
// could commit and that changes what goreleaser builds without the check
// reading it: a goreleaser document other than .goreleaser.yaml, go.work,
// go.work.sum, or anything in vendor. It decides by what `git ls-files
// --cached --others --exclude-standard` lists (gitFiles): a tracked file, even
// one gone from the working tree or added with --force over .gitignore, and an
// untracked file git does not ignore. A developer's ignored go.work never
// reaches the release, which builds from a fresh checkout, so it is not
// refused. Symlinks are listed by git as files and count too.
//
// It fails closed: when git cannot run or cannot list root, it returns an
// error.
func StrayReleaseFiles(ctx context.Context, root string) ([]string, error) {
	names, err := gitFiles(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("find stray release files under %s: %w", root, err)
	}

	var messages []string

	for _, stray := range strayReleaseFiles {
		if slices.ContainsFunc(names, func(name string) bool {
			return name == stray.name || strings.HasPrefix(name, stray.name+"/")
		}) {
			messages = append(messages, stray.name+" is one git could commit: "+stray.reason)
		}
	}

	return messages, nil
}
