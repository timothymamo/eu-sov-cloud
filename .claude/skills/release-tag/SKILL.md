---
name: release-tag
description: Cut a semantic-version release of eu-sov-cloud by tagging main, which triggers the GoReleaser workflow. Use when asked to release, publish a version, or tag a release.
---

# Cut a release

Releases are built by `.github/workflows/release.yml` when a `vX.Y.Z` tag is pushed: GoReleaser
publishes the binaries, then the `image`, `sbom` and `sign` jobs the signed container image to
`ghcr.io/timothymamo/eusov` (ADR 0014). The run's first check fails, and nothing is published, unless
the tagged commit is on `main`: always tag a commit of `main`.

1. In the repository root, which always stays on `main` (check
   `git -C <repository root> branch --show-current`, and stop if it is not), run
   `git -C <repository root> pull --ff-only`. The tree must be clean and CI on `main` green
   (`gh run list --branch main --limit 3`). Tagging commits nothing, so it is the one task that runs here:
   every git command in this flow is `git -C <repository root> ...`.
2. Find the last tag: `git -C <repository root> describe --tags --abbrev=0` (if there is none, start at
   `v0.1.0`).
3. Choose the next version from the commits since then
   (`git -C <repository root> log --oneline <last>..HEAD`):
   breaking change (`!` or `BREAKING CHANGE`) means minor while on `v0.x` and major from `v1`;
   `feat` means minor; only `fix` or `perf` means patch. Changes that only touch docs, tests or chores do
   not need a release.
4. State the version and the reasoning in your report **and wait for explicit confirmation** before
   tagging. A pushed tag publishes binaries and a container image.
5. `git -C <repository root> tag -a vX.Y.Z -m "vX.Y.Z"` and
   `git -C <repository root> push origin vX.Y.Z`.
6. Watch the run (`gh run watch`), then report the release URL (`gh release view vX.Y.Z --json url`).
   On failure, report the failed step's log and stop; never delete or move a pushed tag.
7. Check the image with the README's "Verify the image" commands for the new tag (cosign v3 or later),
   by the digest the tag resolves to. After the very first
   push GHCR has created the package private: report that a maintainer must make
   `ghcr.io/timothymamo/eusov` public once in the package settings (an agent cannot), and that
   `docker pull` fails anonymously until then.
