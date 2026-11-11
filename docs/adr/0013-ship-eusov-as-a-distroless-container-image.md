# 0013. Ship eusov as a distroless container image, built but not yet published

- Status: Accepted
- Date: 2026-11-11

## Context

`eusov` ships as GoReleaser archives and `make build`. Running it means downloading the binary for the
platform or having a Go toolchain. A container image lets people run `eusov assess` with `docker run`,
pinned to a version, in CI pipelines and on hosts with no Go (issue #55).

Forces:

- **Small and inert.** A smaller image pulls faster and has less inside it to patch or attack. The
  binary is pure Go (`CGO_ENABLED=0`), so it needs no libc.
- **TLS later.** The live sources (AWS, Azure, GCP) will call cloud APIs over TLS and need CA
  certificates in the image.
- **Least privilege.** The image must run as non-root and with a read-only root filesystem.
- **No data in layers.** Real inventories and reports (CLAUDE.md rule 2) and credentials (rule 3) must
  never reach a layer, the build stage's included.
- **One build.** The image's binary is built like the release binary: the same Go toolchain version
  and flags. It is not byte-identical: the build context leaves `.git` out, so `go build` stamps no
  `vcs.*` build info, which the GoReleaser build does.
- **Reproducible bases.** Base images are pinned and updated by Renovate (rule 7).

## Decision

A multi-stage `Dockerfile` at the root builds `./cmd/eusov` in the `golang` image and copies only the
binary into `gcr.io/distroless/static-debian13:nonroot` (Debian 13, trixie). CI builds it for both Linux release platforms
and smoke-tests it on every pull request; publishing it to a registry is a follow-up.

- **Final stage: distroless static, `nonroot` tag.** It holds CA certificates, tzdata, an `/etc/passwd`
  with the `nonroot` user (65532) and nothing to run but the binary: no shell, no package manager.
  `USER nonroot:nonroot`, `ENTRYPOINT ["/eusov"]` in exec form, `WORKDIR /work` for the mounted
  inventory, and the OCI labels `source`, `version`, `revision`, `licenses`.
- **Builder: `golang:<go.mod version>-alpine`.** The tag is the exact Go version in `go.mod`.
  It runs on `$BUILDPLATFORM` and cross-compiles with `GOOS=$TARGETOS GOARCH=$TARGETARCH`, so
  `docker buildx --platform linux/amd64,linux/arm64` builds both without emulating the compiler. The
  final stage has no `RUN`, so nothing executes on the target platform either: the arm64 build needs no
  QEMU, and CI installs none (no privileged `binfmt` container). The builder's libc does not matter, as
  the binary links none; alpine is the smallest to pull.
- **Same build as the release.** `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w"` and the
  `internal/version` `-X` flags, as in `.goreleaser.yaml`. `test/container` reads those settings from
  `.goreleaser.yaml` and fails when the `Dockerfile` drifts from them, or from the `go.mod` version.
- **Versioning from git.** `VERSION`, `COMMIT` and `DATE` are build args, defaulting to the values in
  `internal/version`. `make image` passes the values `make build` computes and tags `eusov:<version>`,
  `eusov:dev` and `eusov:latest`, locally only, so `docker run ... eusov` finds it; CI passes the same.
- **Digest pinning.** Every base image is `image:tag@sha256:<digest>`, the digest of the multi-arch
  index, so both platforms resolve and a rebuild uses the same bytes. The tag stays in the reference,
  so Renovate's `dockerfile` manager (enabled by `config:recommended`) updates tag and digest together.
  `test/container` fails on an unpinned `FROM`. The BuildKit image CI's `setup-buildx-action` runs is
  pinned the same way, through a `# renovate:` comment the shared regex manager reads with its digest.
- **Nothing in layers but the binary.** Modules and the build cache are BuildKit cache mounts, never
  layer content. `.dockerignore` is an allow list (`go.mod`, `go.sum`, `cmd/`, `internal/`) minus tests,
  `testdata` and any inventory or report file, with `.git`, `.worktrees`, `bin`, coverage and credential
  files also named explicitly. The final stage copies `/eusov` and nothing else.
- **Distribution: not yet.** This change builds the image in CI without pushing it. Publishing to GHCR
  from `release.yml` (GoReleaser `dockers` or `ko`), signing (cosign) and SBOM or provenance
  attestations are a follow-up issue: they need `packages: write`, an identity for signing and a review
  of the release workflow's closed step list (`check.ReleaseWorkflow`).

## Alternatives considered

- **`scratch` as the final stage.** Smaller, but the CA bundle (and tzdata, if the binary ever needs
  it) and an `/etc/passwd` entry for the user would have to be copied from the builder by hand, and
  kept current with it. distroless maintains them; it is built from Debian packages that its own
  scanners track. Measured in CI (run 43029400887, linux/amd64): the `eusov` binary is 3.17 MB and the
  distroless `static-debian12` layers about 5.5 MB, for about 8.6 MB uncompressed. tzdata is most of
  the base (4.24 MB), then base-files (0.58 MB) and the CA bundle (0.32 MB), the rest under 0.2 MB.
  `scratch` would save roughly those 5.5 MB, less what has to be copied in by hand; on an image this
  size that does not outweigh maintaining the files ourselves.
- **distroless `static` without `nonroot`, or `USER 65532:65532`.** Works the same; the `nonroot` tag
  makes the default user non-root even if `USER` is dropped, and names the home directory credentials
  are mounted into (`/home/nonroot`).
- **`debian:bookworm-slim` or `alpine` final stage.** Brings a shell and a package manager, tens of MB
  more and more CVEs to triage, for nothing the binary uses.
- **`golang:<version>-bookworm` builder.** Equivalent output, larger pull.
- **Build the image with GoReleaser (`dockers`/`ko`) only.** Ties the image to a release tag; a
  `Dockerfile` also builds locally and on every pull request. GoReleaser can reuse this `Dockerfile`
  when publishing lands.
- **Publish to GHCR now.** Widens this change into the release workflow and signing; split out to keep
  the reviewable surface small.

## Consequences

- `docker build .` and `make image` produce a working image; CI fails a pull request whose `Dockerfile`
  does not build for both platforms, does not run read-only as non-root, or cannot assess the e2e
  inventory fixture.
- A Go version bump in `go.mod` must bump the builder's tag and digest too; `test/container` fails
  until it does. Renovate puts the `golang` image in the "Go toolchain" group with the `go` directive
  and `dev/lima.yaml`, so both move in one PR.
- A new directory the build needs at the root must be added to the `.dockerignore` allow list.
- Live sources will read credentials through the SDK default chain from `-e` variables or a read-only
  mount at `docker run` time; the image never contains them.
- Follow-up: publish to GHCR from the release, signed, with an SBOM.
