---
name: release-tag
description: Cut a semantic-version release of eu-sov-cloud by tagging main, which triggers the GoReleaser workflow. Use when asked to release, publish a version, or tag a release.
---

# Cut a release

Releases are built by `.github/workflows/release.yml` (GoReleaser) when a `vX.Y.Z` tag is pushed.

1. In the repository root, which always stays on `main` (check `git branch --show-current`, and stop if
   it is not), run `git pull --ff-only`. The tree must be clean and CI on `main` green
   (`gh run list --branch main --limit 3`). Tagging commits nothing, so it is the one task that runs here.
2. Find the last tag: `git describe --tags --abbrev=0` (if there is none, start at `v0.1.0`).
3. Choose the next version from the commits since then (`git log --oneline <last>..HEAD`):
   breaking change (`!` or `BREAKING CHANGE`) means minor while on `v0.x` and major from `v1`;
   `feat` means minor; only `fix` or `perf` means patch. Changes that only touch docs, tests or chores do
   not need a release.
4. State the version and the reasoning in your report **and wait for explicit confirmation** before
   tagging. A pushed tag publishes binaries.
5. `git tag -a vX.Y.Z -m "vX.Y.Z"` and `git push origin vX.Y.Z`.
6. Watch the run (`gh run watch`), then report the release URL (`gh release view vX.Y.Z --json url`).
   On failure, report the failed step's log and stop; never delete or move a pushed tag.
