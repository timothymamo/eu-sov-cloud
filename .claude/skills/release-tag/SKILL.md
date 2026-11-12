---
name: release-tag
description: Cut a semantic-version release of shift2eu by tagging main, which triggers the GoReleaser workflow. Use when asked to release, publish a version, or tag a release.
---

# Cut a release

Releases are built by `.github/workflows/release.yml` when a `vX.Y.Z` tag is pushed: GoReleaser
publishes the binaries, then the `image`, `sbom` and `sign` jobs the signed container image to
`ghcr.io/timothymamo/shift2eu` (ADR 0015). The run's first check fails, and nothing is published, unless
the tagged commit is on `main`: always tag a commit of `main`.

1. In the repository root, which always stays on `main` (check
   `git -C <repository root> branch --show-current`, and stop if it is not), run
   `git -C <repository root> pull --ff-only`. The tree must be clean and CI on `main` green
   (`gh run list --branch main --limit 3`). Tagging commits nothing, so it is the one task that runs here:
   every git command in this flow is `git -C <repository root> ...`.
2. Confirm the `v*` tag ruleset exists before tagging; stop if it does not (ADR 0015). The ancestry
   guard in `release.yml` only catches mistakes: the run uses the tagged commit's workflow file, so the
   ruleset is what keeps anyone else from publishing a signed image. From the repository root, list the
   rulesets with `gh api repos/{owner}/{repo}/rulesets`, then read each active one with `target` `tag`
   (`gh api repos/{owner}/{repo}/rulesets/<id>`): one must cover `v*` tags in
   `conditions.ref_name.include` (`refs/tags/v*`, `refs/tags/v*.*.*` or `~ALL`) and restrict creation,
   update and deletion (rules `creation`, `update`, `deletion`) with only maintainers bypassing it. If
   none does, report that a maintainer must create it (an agent must not) and stop.
3. Find the last tag: `git -C <repository root> describe --tags --abbrev=0` (if there is none, start at
   `v0.1.0`).
4. Choose the next version from the commits since then
   (`git -C <repository root> log --oneline <last>..HEAD`):
   breaking change (`!` or `BREAKING CHANGE`) means minor while on `v0.x` and major from `v1`;
   `feat` means minor; only `fix` or `perf` means patch. Changes that only touch docs, tests or chores do
   not need a release.
5. State the version and the reasoning in your report **and wait for explicit confirmation** before
   tagging. A pushed tag publishes binaries and a container image.
6. `git -C <repository root> tag -a vX.Y.Z -m "vX.Y.Z"` and
   `git -C <repository root> push origin vX.Y.Z`. Push one tag at a time, and wait for its release run
   to finish before pushing the next: GitHub keeps one pending run in the `release` concurrency group,
   so a newer tag cancels a run still waiting (ADR 0015).
7. Watch the run (`gh run watch`), then report the release URL (`gh release view vX.Y.Z --json url`).
   If a pending run was cancelled, re-run it with `gh run rerun <id>`. On failure, report the failed
   step's log and stop; never delete or move a pushed tag.
8. Check the published image yourself; do not hand this to the user. The Lima VM has cosign, a `gh`
   with `gh attestation`, `crane` and rootless `nerdctl` (ADR 0017); this is the README's "Verify the
   image" in its VM form, for the new tag. Run it from the repository root, with no registry login, and
   call each tool by its absolute path: `~/.local/bin` and `~/go/bin` come first on the `PATH` and the VM
   user can write there, while these binaries are root-owned:

   ```bash
   TAG=vX.Y.Z
   DIGEST="$(/usr/local/bin/crane digest "ghcr.io/timothymamo/shift2eu:$TAG")"
   IMAGE="ghcr.io/timothymamo/shift2eu@$DIGEST"
   /usr/local/bin/cosign verify "$IMAGE" \
     --certificate-identity "https://github.com/timothymamo/shift2eu/.github/workflows/release.yml@refs/tags/$TAG" \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     --certificate-github-workflow-trigger push
   /usr/local/bin/gh attestation verify "oci://$IMAGE" --repo timothymamo/shift2eu \
     --signer-workflow timothymamo/shift2eu/.github/workflows/release.yml \
     --source-ref "refs/tags/$TAG" --deny-self-hosted-runners
   /usr/local/bin/gh attestation verify "oci://$IMAGE" --repo timothymamo/shift2eu \
     --signer-workflow timothymamo/shift2eu/.github/workflows/release.yml \
     --source-ref "refs/tags/$TAG" --deny-self-hosted-runners \
     --predicate-type https://spdx.dev/Document/v2.3
   /usr/local/nerdctl/bin/nerdctl run --rm --read-only --network none "$IMAGE" version   # prints X.Y.Z, without the v
   ```

   Report the digest and each command's outcome. Every command must exit 0 and `version` must print the
   tag without its `v`; if one fails, report its output and stop (never re-tag). After the very first
   push GHCR has created the package private, so `crane digest` and the pull fail anonymously: report
   that a maintainer must make `ghcr.io/timothymamo/shift2eu` public once in the package settings (an agent
   cannot), and run the checks after that.

   This is a smoke test, not the trust anchor: every agent shares the VM user, which has passwordless
   `sudo`, so any of them could tamper with these binaries or their output. An independent check on a
   GitHub-hosted runner, a post-publish job in `release.yml`, is the follow-up that would make it
   authoritative (ADR 0017).
