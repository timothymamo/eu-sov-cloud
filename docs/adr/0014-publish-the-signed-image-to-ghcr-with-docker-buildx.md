# 0014. Publish the signed container image to GHCR from a release job of its own, with docker buildx

- Status: Accepted
- Date: 2026-11-11

## Context

ADR 0013 added a distroless image that CI builds for `linux/amd64` and `linux/arm64` on every pull
request, and left publishing it as a follow-up (issue #72). People still have to clone the repository and
build the image; there is nothing to `docker pull`, pin to a version, or verify as coming from this
repository's release.

Forces:

- **One build.** The published image must be the `Dockerfile`'s: its builder stage is what
  `test/container` pins to `go.mod` and `.goreleaser.yaml`, and its final stage is the distroless choice
  of ADR 0013. `eusov version` in the image must report what the release binary reports.
- **Least privilege per job.** Pushing needs `packages: write`, keyless signing an OIDC token
  (`id-token: write`), and GitHub attestations `attestations: write`. The goreleaser job already holds
  `contents: write` (it creates the release); one job holding all of them would let a compromise of
  either step use both.
- **A closed release workflow.** `check.ReleaseWorkflow` reads `release.yml` as a closed schema (ADR
  0012): every job and step is listed, no env or build cache is carried in, and no `GO*`/`CGO*` names or
  env files appear. The new job must fit that model.
- **Verifiable.** A user must be able to check that an image was built by this repository's release
  workflow from a tag, and see what it contains, with standard tools.
- **Pinned and updated by Renovate** (CLAUDE.md rule 7): every action by commit, every tool by exact
  version.

## Decision

A third job, `image`, in `.github/workflows/release.yml`, `needs: test`, builds the `Dockerfile` with
`docker/build-push-action` on a pinned BuildKit, pushes the multi-arch index to
`ghcr.io/timothymamo/eusov`, signs it with cosign (keyless, GitHub OIDC), and attaches build provenance
and an SPDX SBOM as GitHub attestations, pushed to the registry beside the image.

- **Permissions**, job level, with the workflow's default still `{}`: `contents: read` (checkout),
  `packages: write` (push the image, its signature and attestations), `id-token: write` (the OIDC token
  for cosign and the attestations' Sigstore certificate), `attestations: write` (store the attestations
  in the repository). The goreleaser job keeps `contents: write` alone.
- **Steps**, a closed list: `actions/checkout` (full history, no persisted credentials); a `run:` step
  computing the build arguments; `docker/setup-buildx-action` with the BuildKit image pinned by tag and
  digest in `driver-opts` (no QEMU, as ADR 0013 explains); `docker/login-action` to `ghcr.io` with the
  job's `GITHUB_TOKEN`; `docker/metadata-action` for the tags; `docker/build-push-action`;
  `sigstore/cosign-installer` and `cosign sign --yes ghcr.io/timothymamo/eusov@<digest>`;
  `actions/attest-build-provenance`; `anchore/sbom-action`; `actions/attest-sbom`.
- **Tags.** The git tag (`vX.Y.Z`), `X.Y`, and `latest`, from `metadata-action`'s semver rules with
  `latest=auto`. A pre-release tag (`v0.1.0-rc.1`) gets only its own tag: no `X.Y`, no `latest`.
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
  `build-push-action` returns. `attest-build-provenance` and `attest-sbom` attest that digest and push
  the bundles to the registry (`push-to-registry: true`); `create-storage-record: false`, as storage
  records are an organisation API and would need `artifact-metadata: write`. The SBOM is SPDX JSON from
  syft (`anchore/sbom-action`, exact `syft-version`), read from the registry by digest, uploaded neither
  to the run nor to the release (that would need `contents: write`). syft reads the index for the
  runner's platform, `linux/amd64`; the arm64 image holds the same Go modules and the same distroless
  package versions.
- **BuildKit's own attestations off** (`provenance: false`, `sbom: false`). They would add unsigned
  attestation manifests to the index (shown by GHCR as an `unknown/unknown` platform) that duplicate,
  less trustworthily, the signed GitHub attestations. There is one signed source of provenance and
  SBOM, and it is the one `gh attestation verify` checks.
- **Pins.** Every action by full commit SHA with its version in a comment (Renovate's `github-actions`
  manager). cosign (`cosign-release`), syft (`syft-version`) and the BuildKit image carry a
  `# renovate:` comment; the shared regex manager reads a value of the form `image=<name>:<tag>@<digest>`
  (setup-buildx's `driver-opts`) since this change.
- **Dry run on every pull request.** CI's `image` job runs the same tag rules on a sample release and
  pre-release tag and checks the result, builds the multi-arch image with the same settings into an OCI
  archive, and generates and checks the SBOM from it. Pushing, signing and attesting need the OIDC token
  and write access, so only a tag exercises them; the first tag, ideally a `-rc` pre-release, is verified
  by hand with the README's commands.
- **Public package.** GHCR creates a new package private. After the first push it is made public once,
  by hand, in the package settings; later pushes keep it public.

## Alternatives considered

- **GoReleaser `dockers` / `dockers_v2` with `docker_manifests`.** GoReleaser copies its own prebuilt
  binaries into the build context, so the `Dockerfile`'s builder stage, which ADR 0013 and
  `test/container` pin, would not run; a second, release-only Dockerfile would drift from the tested one.
  It would also put `packages: write` and `id-token: write` in the job that holds `contents: write`.
- **`ko`.** Builds Go images without a `Dockerfile` on its own base image choice; it ignores the
  `Dockerfile` and the distroless `nonroot` final stage this project tested and documented.
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

- `docker pull ghcr.io/timothymamo/eusov:<version>` works for every release from the first tag on, for
  both platforms; `cosign verify` and `gh attestation verify` check signature, provenance and SBOM. The
  README documents the exact commands.
- `check.ReleaseWorkflow` must list the `image` job, its permissions and its steps (a later change in
  this issue); until it does, `test/architecture` fails on the real `release.yml`.
- The release binaries' `eusov version` now reports the commit date, not the build time.
- A failed `image` job does not undo the GitHub release the goreleaser job created, and vice versa; the
  jobs run in parallel after `test`. A failure is fixed forward with a new tag.
- After the first push the GHCR package must be made public by hand, once.
- New third-party actions to keep updated: `docker/login-action`, `docker/metadata-action`,
  `sigstore/cosign-installer`, `actions/attest-build-provenance`, `anchore/sbom-action`,
  `actions/attest-sbom`; Renovate groups them with the other actions.
