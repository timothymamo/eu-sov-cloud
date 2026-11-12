# Verify a release

Every published image is signed and carries build provenance and an SBOM. This page shows how to check
all three for a release before you run it, inside the project's Lima VM too, how to check a release
archive against its checksums, and how to verify the releases published before the project's rename.

## Verify the image

Every published image is signed with [cosign](https://github.com/sigstore/cosign), keyless, by the
release workflow's GitHub identity for that tag, and carries build provenance and an SPDX SBOM as
GitHub attestations. You need cosign v3 or later, which reads the signature from the registry in the
bundle format the release writes, and the [GitHub CLI](https://cli.github.com/). The release
workflow's `verify` job runs these same checks on every release, on a fresh GitHub-hosted runner with
read-only permissions, and fails the release when one fails. It runs after the images are published, so
it detects a bad release rather than prevents one; run the checks yourself before you trust an image.

A tag can be moved; a digest cannot. Resolve the release tag to its digest once, then verify and run
that digest:

```bash
TAG=v0.2.0
DIGEST="$(docker buildx imagetools inspect "ghcr.io/timothymamo/shift2eu:$TAG" --format '{{.Manifest.Digest}}')"
IMAGE="ghcr.io/timothymamo/shift2eu@$DIGEST"

# The signature: made by release.yml, run on a push of exactly this tag.
cosign verify "$IMAGE" \
  --certificate-identity "https://github.com/timothymamo/shift2eu/.github/workflows/release.yml@refs/tags/$TAG" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-github-workflow-trigger push

# The build provenance (SLSA), then the SBOM: from release.yml, for this tag, on GitHub's runners.
gh attestation verify "oci://$IMAGE" --repo timothymamo/shift2eu \
  --signer-workflow timothymamo/shift2eu/.github/workflows/release.yml \
  --source-ref "refs/tags/$TAG" --deny-self-hosted-runners
gh attestation verify "oci://$IMAGE" --repo timothymamo/shift2eu \
  --signer-workflow timothymamo/shift2eu/.github/workflows/release.yml \
  --source-ref "refs/tags/$TAG" --deny-self-hosted-runners \
  --predicate-type https://spdx.dev/Document/v2.3

docker run --rm "$IMAGE" version
```

Inside the project's Lima VM, which has rootless nerdctl and no Docker
([ADR 0017](../adr/0017-run-a-rootless-container-runtime-in-the-lima-vm.md)), resolve the digest with
[crane](https://github.com/google/go-containerregistry/tree/main/cmd/crane), which reads only the
index and pulls nothing, and run with `nerdctl`; the `cosign` and `gh` commands are the same:

```bash
# Replaces the DIGEST= line and the final `docker run` line above; TAG, IMAGE, cosign and gh stay as they are.
DIGEST="$(crane digest "ghcr.io/timothymamo/shift2eu:$TAG")"
nerdctl run --rm --read-only --network none "$IMAGE" version
```

Each platform's image is signed too, so `cosign verify` also accepts the `linux/amd64` or
`linux/arm64` digest that `docker buildx imagetools inspect` lists. `gh attestation verify ...
--format json` prints the attested statements, the SBOM's packages among them. The SBOM is attested
on the index digest but generated from its `linux/amd64` image; the `linux/arm64` image is built from
the same source, Go modules and distroless base (`crane digest --platform linux/arm64 ...` prints one
platform's digest). The image's labels
name the source repository (`org.opencontainers.image.source`) and the commit
(`org.opencontainers.image.revision`).

## Verify an archive

The release archives, the CLI's `shift2eu_*` and the HTTP API's `shift2eu-server_*`
([Install](install.md#the-release-archives)), are listed in one `checksums.txt`, published with the
release. The archives and `checksums.txt` are neither signed nor attested: only the images are. A
matching checksum shows the download is the file the release published, not who published it; for
that, use the images. Check either archive the same way:

```bash
TAG=v0.2.0
VERSION="${TAG#v}"
ARCHIVE="shift2eu_${VERSION}_linux_amd64.tar.gz"   # the HTTP API's: shift2eu-server_${VERSION}_linux_amd64.tar.gz

gh release download "$TAG" --repo timothymamo/shift2eu --pattern "$ARCHIVE" --pattern checksums.txt
sha256sum --check --ignore-missing checksums.txt   # macOS: shasum -a 256 --check --ignore-missing checksums.txt
tar -xzf "$ARCHIVE"
```

`sha256sum` must print `OK` for the archive. Each archive holds only its own binary (`shift2eu` or
`shift2eu-server`), with `README.md`, `LICENSE` and `docs/`. The `shift2eu-server_*` archives start
with the first release after `v0.2.0`; up to `v0.2.0`, the `shift2eu_*` archive holds both binaries.

## Releases before the rename: v0.1.0 and v0.1.0-rc.1

The project was called `eu-sov-cloud` until v0.2.0
([ADR 0020](../adr/0020-rename-the-project-to-shift2eu.md)). `v0.1.0` and `v0.1.0-rc.1` keep the
names they were published with: binaries `eusov_*`, image `ghcr.io/timothymamo/eusov`, and the
signing identity of the repository's old name. Verify them with the commands above, with these
changes:

```bash
TAG=v0.1.0   # or v0.1.0-rc.1
DIGEST="$(docker buildx imagetools inspect "ghcr.io/timothymamo/eusov:$TAG" --format '{{.Manifest.Digest}}')"
IMAGE="ghcr.io/timothymamo/eusov@$DIGEST"

cosign verify "$IMAGE" \
  --certificate-identity "https://github.com/timothymamo/eu-sov-cloud/.github/workflows/release.yml@refs/tags/$TAG" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-github-workflow-trigger push

gh attestation verify "oci://$IMAGE" --repo timothymamo/eu-sov-cloud \
  --signer-workflow timothymamo/eu-sov-cloud/.github/workflows/release.yml \
  --source-ref "refs/tags/$TAG" --deny-self-hosted-runners
```

Add `--predicate-type https://spdx.dev/Document/v2.3` to the `gh attestation verify` line for the
SBOM, as above. GitHub redirects the old repository name to the new one.
