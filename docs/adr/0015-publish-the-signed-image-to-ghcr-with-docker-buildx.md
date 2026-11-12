# 0015. Publish the signed container image to GHCR from release jobs of their own, with docker buildx

- Status: Accepted. Amended by issue #92: a pre-release tag is a GitHub pre-release too (Tags, below).
  Amended by PRD #103: a second image, `shift2eu-server`, is published the same way (Consequences).
  Amended by issue #99: an independent `verify` job checks both published images (Consequences).
- Date: 2026-11-11

## Context

ADR 0013 added a distroless image that CI builds for `linux/amd64` and `linux/arm64` on every pull
request, and left publishing it as a follow-up (issue #72). People still have to clone the repository and
build the image; there is nothing to `docker pull`, pin to a version, or verify as coming from this
repository's release.

Forces:

- **One build.** The published image must be the `Dockerfile`'s: its builder stage is what
  `test/container` pins to `go.mod` and `.goreleaser.yaml`, and its final stage is the distroless choice
  of ADR 0013. `shift2eu version` in the image must report what the release binary reports.
- **Least privilege per job.** Pushing needs `packages: write`, keyless signing an OIDC token
  (`id-token: write`), and GitHub attestations `attestations: write`. The goreleaser job already holds
  `contents: write` (it creates the release); one job holding all of them would let a compromise of
  either step use both. The signing identity is the most valuable of them: whatever holds it can
  publish an image that verifies as this repository's release, so only first-party and Sigstore code
  may run next to it.
- **A closed release workflow.** `check.ReleaseWorkflow` reads `release.yml` as a closed schema (ADR
  0012): every job and step is listed, no env or build cache is carried in, and no `GO*`/`CGO*` names or
  env files appear. The new job must fit that model.
- **Verifiable.** A user must be able to check that an image was built by this repository's release
  workflow from a tag, and see what it contains, with standard tools.
- **Pinned and updated by Renovate** (CLAUDE.md rule 7): every action by commit, every tool by exact
  version.

## Decision

Three jobs in `.github/workflows/release.yml` publish the image, after `test` and `goreleaser`. `image`
builds the `Dockerfile` with `docker/build-push-action` on a pinned BuildKit and pushes the multi-arch
index to `ghcr.io/timothymamo/shift2eu`. `sbom` generates an SPDX SBOM of the pushed index. `sign` signs
the index with cosign (keyless, GitHub OIDC) and attaches build provenance and the SBOM as GitHub
attestations, pushed to the registry beside the image.

- **Only a commit on `main` is released, by mistake-proofing.** The `test` job checks out the whole
  history and first runs `git merge-base --is-ancestor "$GITHUB_SHA" origin/main`, failing if
  `origin/main` is missing from the checkout. Every other job needs `test`, so a `v*` tag pushed by
  mistake on a commit that is not on `main` publishes neither binaries nor an image. The guard is not a
  security boundary: GitHub runs the workflow file of the tagged commit, so anyone who may push a `v*`
  tag can tag a branch whose `release.yml` drops the guard, and get an image signed with this
  repository's release identity. Only the tag ruleset below keeps that out.
- **Publishing waits for the release.** `image` needs `test` and `goreleaser`: a failed GoReleaser run
  publishes no image.
- **Three jobs, split by what they may do and what they run.** Job-level permissions, with the
  workflow's default still `{}`:
  - `image`: `contents: read` (checkout) and `packages: write` (push). No OIDC token, so nothing in it
    can sign, although it runs the `docker/*` actions. Its output is the index digest.
  - `sbom`: `packages: read` only, to pull the image while the package is still private. It runs
    `anchore/sbom-action`, which downloads the third-party syft binary; a compromised syft can neither
    push nor sign here, at worst write a false SBOM that the next job attests. The SBOM leaves the job
    as a workflow artifact (`actions/upload-artifact`, kept one day).
  - `sign`: `packages: write` (push the signatures and attestations), `id-token: write` (the OIDC
    token for cosign and the attestations' Sigstore certificate), `attestations: write` (store the
    attestations in the repository). No checkout, no `docker/*` and no `anchore` action: only
    `sigstore/cosign-installer` and cosign, and GitHub's `actions/download-artifact` and
    `actions/attest`. It logs in with `cosign login ghcr.io --password-stdin`, fed the job's
    `GITHUB_TOKEN` through the step's `env:`, which writes the credential where `actions/attest` reads
    it, so no third-party login action runs there.

  The goreleaser job keeps `contents: write` alone.
- **Steps**, a closed list. `image`: `actions/checkout` (full history and tags, no persisted
  credentials); a `run:` step computing the build arguments and the `latest` decision;
  `docker/setup-buildx-action` with the BuildKit image pinned by tag and digest in `driver-opts` (no
  QEMU, as ADR 0013 explains); `docker/login-action` to `ghcr.io` with the job's `GITHUB_TOKEN`;
  `docker/metadata-action` for the tags; `docker/build-push-action`. `sbom`: `docker/login-action`;
  `anchore/sbom-action`; `actions/upload-artifact`. `sign`: `sigstore/cosign-installer`; `cosign login`;
  `cosign sign --yes --recursive ghcr.io/timothymamo/shift2eu@<digest>`; `actions/attest` (provenance);
  `actions/download-artifact`; `actions/attest` (SBOM).
- **Tags.** The git tag (`vX.Y.Z`) and `X.Y` from `metadata-action`'s semver rules. `latest` only when
  the tag is the highest release tag on `main`: the build-arguments step sorts
  `git tag -l 'v*.*.*' --merged refs/remotes/origin/main` by version, leaving out pre-releases, and
  passes `true` or `false` to `metadata-action` (`flavor: latest=...`). A patch to
  an older line (`v1.2.4` after `v1.3.0`) does not move `latest`, which `latest=auto` would. A
  pre-release tag (`v0.1.0-rc.1`) gets only its own tag: no `X.Y`, no `latest`. The GitHub release
  follows the same rule (issue #92): `.goreleaser.yaml` sets `release.prerelease: auto`, so a tag with
  a pre-release suffix is published as a GitHub pre-release, never the latest release, and a plain
  `vX.Y.Z` as a normal one; `check.GitHubRelease` pins that section to exactly that one key. Counting only tags
  merged into `main` keeps a stray tag off `main`, one the guard refused and nobody deleted, from
  outranking every later release and freezing `latest`.
- **One release at a time.** The workflow's `concurrency` group `release`, with
  `cancel-in-progress: false`, queues a tag's run while another runs. Two tags pushed together would
  otherwise race on `latest`, and whichever run pushed last would win, not the highest tag. A run in
  progress is never cancelled, so no release stops half published. GitHub keeps one pending run per
  group: a third tag pushed while one run waits replaces the waiting one, which is then re-run by hand.
- **Same stamp as the release binaries.** `VERSION` is the tag without its `v` (GoReleaser's
  `.Version`), `COMMIT` is `git rev-parse --short HEAD` on a full clone (`.ShortCommit`: git picks the
  short length from the object count, so both jobs clone the whole history), and `DATE` the commit date
  in RFC 3339, UTC. GoReleaser's `.Date` is the time GoReleaser runs, which another job cannot know, so
  `.goreleaser.yaml` now stamps `.CommitDate` instead: both builds report the same date, and a rebuild
  of a tag reports the same date too. Tag-derived values reach the `run:` script through the step's
  `env:`, never through `${{ }}` inside the script.
- **No cache in.** No `cache-from`/`cache-to`: BuildKit on a fresh runner starts empty, as setup-go's
  `cache: false` does for the goreleaser job. The `Dockerfile`'s cache mounts live only for that build.
- **Labels from the `Dockerfile` only.** `metadata-action`'s labels are not passed, so the
  `Dockerfile`'s OCI labels (`source`, `version`, `revision`, ...) are the image's. The index gets the
  `org.opencontainers.image.source` annotation as well, so GHCR links the multi-arch package to the
  repository.
- **Signed and attested by digest, never by tag.** cosign signs the index digest that
  `build-push-action` returns, with `--recursive`, so each platform's manifest is signed as well and a
  per-platform digest verifies too. cosign v3 stores the signature in the Sigstore bundle format as an
  OCI referrer, so verifying needs cosign v3 or later. `actions/attest` attests that digest twice, the
  SLSA build provenance (its default) and the SBOM (`sbom-path`), and pushes the bundles to the registry
  (`push-to-registry: true`); `create-storage-record: false`, as storage records are an organisation API
  and would need `artifact-metadata: write`. `actions/attest-build-provenance` and `actions/attest-sbom`
  are, from v4, thin wrappers around `actions/attest`, and `attest-sbom` cannot turn the storage record
  off, so the workflow uses `actions/attest` directly. The SBOM is SPDX JSON from syft
  (`anchore/sbom-action`, exact `syft-version`), read from the registry by digest, kept only as a
  one-day workflow artifact and uploaded nowhere else (the release would need `contents: write`). syft
  reads the index for the runner's platform, `linux/amd64`; the arm64 image holds the same Go modules
  and the same distroless package versions.
- **BuildKit's own attestations off** (`provenance: false`, `sbom: false`). They would add unsigned
  attestation manifests to the index (shown by GHCR as an `unknown/unknown` platform) that duplicate,
  less trustworthily, the signed GitHub attestations. There is one signed source of provenance and
  SBOM, and it is the one `gh attestation verify` checks.
- **Pins.** Every action by full commit SHA with its version in a comment (Renovate's `github-actions`
  manager). cosign (`cosign-release`), syft (`syft-version`) and the BuildKit image carry a
  `# renovate:` comment; the shared regex manager reads a value of the form `image=<name>:<tag>@<digest>`
  (setup-buildx's `driver-opts`) since this change.
- **Dry run on every pull request.** CI's `image` job runs the same `latest` decision on sample tag
  lists, standing for the tags on `main` that `git tag -l --merged` lists in the release (one where
  the sample release is the highest, one where it is not, one pre-release), feeds the results to the
  same tag rules and checks the tags, builds the multi-arch image with the same settings into an OCI
  archive, and generates and checks the SBOM from it. Pushing, signing and attesting need the OIDC token
  and write access, so only a tag exercises them; the first tag, ideally a `-rc` pre-release, is verified
  by hand with the README's commands.
- **Public package.** GHCR creates a new package private. After the first push it is made public once,
  by hand, in the package settings; later pushes keep it public.
- **Required repository setting: a `v*` tag ruleset.** Who may push a `v*` tag decides who can publish
  under the release identity, and that is a repository setting, not code. A tag ruleset on
  `refs/tags/v*` restricts creation, update and deletion to maintainers (optionally requiring signed
  commits). It is set by hand and is the security boundary; the ancestry guard only catches mistakes.
  It is a precondition of every release: the `release-tag` skill checks that it exists
  (`gh api repos/{owner}/{repo}/rulesets`) and stops before tagging if it does not.
- **Verification instructions** pin an exact identity per release: `cosign verify` with
  `--certificate-identity .../release.yml@refs/tags/vX.Y.Z` and the GitHub OIDC issuer, and
  `gh attestation verify` with `--source-ref refs/tags/vX.Y.Z`, `--signer-workflow` and
  `--deny-self-hosted-runners`, on the digest the tag resolves to once.

## Alternatives considered

- **GoReleaser `dockers` / `dockers_v2` with `docker_manifests`.** GoReleaser copies its own prebuilt
  binaries into the build context, so the `Dockerfile`'s builder stage, which ADR 0013 and
  `test/container` pin, would not run; a second, release-only Dockerfile would drift from the tested one.
  It would also put `packages: write` and `id-token: write` in the job that holds `contents: write`.
- **`ko`.** Builds Go images without a `Dockerfile` on its own base image choice; it ignores the
  `Dockerfile` and the distroless `nonroot` final stage this project tested and documented.
- **One publishing job holding push, OIDC token and attestation permissions** (this ADR's first
  version). Fewer jobs, but the third-party syft binary and the `docker/*` actions run next to the
  signing identity: any of them, compromised, could push and keyless-sign an image under this
  repository's release identity.
- **`docker/login-action` in the sign job.** It works, but is one more third-party action next to the
  signing identity; `cosign login` does the same with code the job runs anyway.
- **Push from the goreleaser job.** One job fewer, but every write permission in one place, and
  `check.ReleaseWorkflow` would have to allow the goreleaser job's runner state (docker login config,
  BuildKit) next to goreleaser.
- **BuildKit `provenance: mode=max` and `sbom: true` instead of GitHub attestations.** No extra actions,
  but the attestations are unsigned unless signed separately, and `gh attestation verify` does not read
  them.
- **cosign `attest` for SBOM and provenance.** Works, but the provenance would be ours to write; the
  `actions/attest-*` actions produce SLSA provenance from the run itself and make it visible in the
  repository's attestations, verifiable with `gh attestation verify`.
- **Pin BuildKit in a job-level `env:` as `ci.yml` does.** `check.ReleaseWorkflow` forbids job-level
  `env:` in the release workflow; the pin goes into `driver-opts` with the regex manager extended.

## Consequences

- `docker pull ghcr.io/timothymamo/shift2eu:<version>` works for every release from the first tag on, for
  both platforms; `cosign verify` and `gh attestation verify` check signature, provenance and SBOM. The
  README documents the exact commands.
- `check.ReleaseWorkflow` lists the `image`, `sbom` and `sign` jobs, their permissions and their steps.
- `shift2eu version` in the release binaries and in the image now reports the commit date, not the build
  time; a binary from `make build` still reports the time it was built.
- `shift2eu version` prints the version without its leading `v` (`0.1.0` for the tag `v0.1.0`).
- A failed GoReleaser run publishes no image; a failed `image`, `sbom` or `sign` job does not undo the
  GitHub release. A failure is fixed forward with a new tag. An image pushed by a run whose `sign` job
  failed stays unsigned and fails verification.
- A tag on a commit that is not on `main` fails the release run at its first check, and never counts
  for `latest`. Someone who may push `v*` tags can still bypass the check with an edited workflow, so
  no release is cut until the `v*` tag ruleset exists.
- Release runs are serialised; a tag pushed while one runs waits for it.
- After the first push the GHCR package must be made public by hand, once.
- New actions to keep updated: `docker/login-action`, `docker/metadata-action`,
  `sigstore/cosign-installer`, `anchore/sbom-action`, `actions/upload-artifact`,
  `actions/download-artifact`, `actions/attest`; Renovate groups them with the other actions.
- Amended by PRD #103 (milestone 4): the release publishes a second image,
  `ghcr.io/timothymamo/shift2eu-server`, the HTTP API of
  [ADR 0019](0019-serve-assessments-over-a-versioned-http-api-from-a-separate-binary.md), built from the
  `Dockerfile`'s `server` stage (`--target server`) with the same platforms, stamp, tag rules, SBOM,
  signature and attestations. It has jobs of its own, `server-image`, `server-sbom` and `server-sign`, with
  the same split permissions as `image`, `sbom` and `sign`, rather than a matrix over the two images: a
  matrix job's `outputs` keep the value of one leg only, so the sign job could not read each image's
  digest, and one job per image keeps each one's privileges and steps a closed list.
  `check.ReleaseWorkflow` builds both images' jobs from the same step lists, so the two cannot drift, and
  `check.DryRunMirrorsRelease` holds CI's dry run of each image to its release build. GoReleaser builds
  `shift2eu-server` beside `shift2eu` with the same settings, and each release archive holds both
  binaries. The GHCR package `shift2eu-server` must be made public by hand after its first push, as
  `shift2eu`'s was.
- Amended by issue #99: a ninth job, `verify`, checks what every release published, independently of
  the agents. Until then the published images were checked by hand, or by the `release-tag` skill inside
  the agents' Lima VM ([ADR 0017](0017-run-a-rootless-container-runtime-in-the-lima-vm.md)); audit
  finding M1 of PR #96 noted that this check is not independent: the agents that run it share the VM,
  and what runs there can alter the check. `verify` needs `image`, `sign`, `server-image` and
  `server-sign` and runs on a fresh GitHub-hosted runner; every script in it starts with
  `set -euo pipefail`. For each image it resolves the tag to a digest and fails unless that is the
  digest this run's image job built (a tag can be moved between the build and the check, a digest
  cannot), and, by that digest, runs the README's "Verify the image": `cosign verify` with the exact
  release identity (`release.yml@refs/tags/<tag>`, GitHub's OIDC issuer, the `push` trigger) and the
  tagged commit (`--certificate-github-workflow-sha`); `gh attestation verify` of the build provenance
  and of the SPDX SBOM, with the repository, the signer workflow, the tag as source ref, the tagged
  commit as source digest (`--source-digest`) and self-hosted runners denied; and the image itself with
  `docker run --rm --read-only --network none`: the CLI's `version`, which must report the tag's
  version, and the server's `--help`, which must exit 0, since `shift2eu-server` has no `version`
  subcommand. So the signature and both attestations are bound to the tagged commit, not only to the
  tag name. The image run is the runner's platform, `linux/amd64`; `linux/arm64` is checked by its
  signature (cosign signed the index recursively) and its attestations only, never run.

  Then it checks the tag rule above for both images, on the digests this run built: `vX.Y.Z` always;
  `X.Y` for every release; and a pre-release moves neither `X.Y` nor `latest`, so a pre-release fails
  only if `X.Y` or `latest` resolves to its digest (a shipped `X.Y` of an earlier release is fine).
  `latest` follows the highest release tag, which, as in the stamp, is the highest of the release tags
  merged into `main`, read by ancestry: the job fetches `main` and the `v*` tags of the public repository
  treeless (`git init` and `git fetch --filter=tree:0`, no token) into a repository of its own, with no
  checkout, so no repository code runs, and picks the highest with the stamp's filter, sort and pick. Not
  GitHub's "latest release", which is the most recently created release rather than the highest version,
  and not every tag on the remote, which would count a tag off `main`. No release tag merged into `main`
  is an error. When the tag is the highest, `latest` must be its digest. When a higher release's images
  are published, `latest` must not be this digest. When a higher release tag exists but its images are
  not published yet (its own run is still in flight), the `latest` check is skipped with a `::notice::`:
  that window is not covered, so a release that should have moved `latest` and did not passes while the
  newer run is in flight. A re-run of an older release's `verify` after a newer release has shipped
  passes, since `latest` is then the newer release's. A registry error on a tag the rule requires fails
  the job; only the reads whose absence the rule allows are silenced.

  Its permissions are `packages: read` (the images and their signatures) and `attestations: read`
  (`gh attestation verify` reads the attestations through the repository's API), nothing else: no
  `id-token`, so nothing in it can sign, and no `contents`, as it checks out no repository code; its
  only action is `sigstore/cosign-installer`, by commit, with cosign by exact release.

  `verify` runs after the images and tags are published, so it detects a bad release, it does not
  prevent one. A failed `verify` is a failed release, never retried around: the `release-tag` skill
  reports the job's result, and on a failure the maintainer marks the GitHub release as bad (edits its
  notes, or deletes it), points `latest` (and `X.Y` if wrong) back at the last good release's digest or
  deletes the bad image tag, and fixes forward with a new patch release; a published tag and digest are
  never reused. `check.ReleaseWorkflow` pins the job's keys, needs, permissions and steps, and
  `check.DryRunMirrorsRelease` holds CI's dry run to run each image it built with the same
  `docker run` flags and command as `verify`, each image's run on that image's own dry-run build
  (`$IMAGE` for the CLI, `$SERVER_IMAGE` for the server), and refuses a `docker run` line of `verify`
  continued onto the next line, which it could not compare whole; pushing, signing and attesting still
  need a tag, so the rest of `verify` runs only in a release.
