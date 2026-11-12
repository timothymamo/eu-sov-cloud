# 0020. Rename the project to shift2eu, keep published releases and history as they are

- Status: Accepted
- Date: 2026-11-12

## Context

The project started as `eu-sov-cloud`: repository `timothymamo/eu-sov-cloud`, Go module
`github.com/timothymamo/eu-sov-cloud`, binaries `eusov` and `eusov-server`, image
`ghcr.io/timothymamo/eusov`. Issue #116 renames it to `shift2eu` everywhere. Two releases were already
published under the old names, `v0.1.0` and `v0.1.0-rc.1`: archives `eusov_*`, image
`ghcr.io/timothymamo/eusov`, and cosign signatures and GitHub attestations whose identity is
`https://github.com/timothymamo/eu-sov-cloud/.github/workflows/release.yml@refs/tags/<tag>`. That
identity is part of each signature and cannot change after the fact.

GitHub renamed the repository in place (#116 step 2), which keeps history, tags, releases and their
assets, issues, pull requests, settings, rulesets and secrets, and redirects the old name to the new
one for the web, git and the API, until a repository with the old name is created again.

## Decision

Rename every reference in the repository to `shift2eu` in one change (#117):

| Thing | Before | After |
|---|---|---|
| Repository | `timothymamo/eu-sov-cloud` | `timothymamo/shift2eu` |
| Go module | `github.com/timothymamo/eu-sov-cloud` | `github.com/timothymamo/shift2eu` |
| CLI | `eusov` (`cmd/eusov`) | `shift2eu` (`cmd/shift2eu`) |
| HTTP API server | `eusov-server` (`cmd/eusov-server`) | `shift2eu-server` (`cmd/shift2eu-server`) |
| Image | `ghcr.io/timothymamo/eusov` | `ghcr.io/timothymamo/shift2eu` |
| Signing identity | `.../eu-sov-cloud/.github/workflows/release.yml@refs/tags/...` | `.../shift2eu/.github/workflows/release.yml@refs/tags/...` |
| Environment variables | `EUSOV_*` | `SHIFT2EU_*` |
| Lima VM, deck orchestration, workspace | `eu-sov-cloud`, `eu-sov-cloud-ws/eu-sov-cloud` | `shift2eu`, `shift2eu-ws/shift2eu` |

Behaviour does not change. The JSON report keeps `schemaVersion` 1: only the schemas' `$id` URLs move
([ADR 0018](0018-version-the-json-report-format-for-the-cli-and-the-api.md)). The first release with
the new names is `v0.2.0`, a minor bump because the binary and image names change (a breaking change
on `v0.x`).

**Kept as they are:**

- `v0.1.0` and `v0.1.0-rc.1` keep their binary, archive and image names and their signing identity.
  The README's section on those releases is, with this ADR, the only place the old names remain in
  the tree. The old image stays published, with a note pointing at `shift2eu` (#116 step 7).
- Git history, tags and merged pull requests keep the old names. History is not rewritten.
- The old repository name `eu-sov-cloud` is never reused, by this account or any other it controls.

## Alternatives considered

- **Rewrite history to the new names.** Every commit SHA would change: the tags of the published
  releases would no longer point at the commits their signatures and provenance name, every clone and
  open branch would break, and links from issues and pull requests to commits would dangle. Nothing a
  reader gains is worth that.
- **Re-publish v0.1.0 under the new names.** A new signature would claim the new workflow built an
  artifact it did not build, and the same version would exist twice with different identities.
  Verifying the old release with the old identity is honest and costs one README section.
- **Keep the module path or binary names and rename only the repository.** GitHub's redirect would
  keep `go get` working for a while, but the names would disagree with the project's, and the
  redirect lasts only until someone creates a repository with the old name.
- **A new repository instead of a rename.** Issues, pull requests, releases, settings and the tag
  ruleset would have to be moved by hand, and some cannot be.

## Consequences

- Verifying `v0.1.0` and `v0.1.0-rc.1` needs the old image name and identity; the README says how.
- The old GHCR package `ghcr.io/timothymamo/eusov` is kept, deprecated and never deleted: the
  README's commands for verifying v0.1.0 and v0.1.0-rc.1 depend on it.
- `go install` and imports use `github.com/timothymamo/shift2eu`; the old module path resolves only
  through GitHub's redirect and is not supported.
- Reusing the name `eu-sov-cloud` for a new repository would end GitHub's redirect and let that
  repository answer for the old URLs, including the source links in the published v0.1.0 artifacts.
  It must never be reused.
- The Lima VM is provisioned with the old names until it is re-created after this change merges
  (#116 step 5): `dev/lima.yaml` applies only at creation.
- `SHIFT2EU_SERVER_TOKEN` replaces `EUSOV_SERVER_TOKEN`. `shift2eu-server` has not been released, so no
  deployment reads the old name.
