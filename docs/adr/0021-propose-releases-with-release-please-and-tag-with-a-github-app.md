# 0021. Propose releases with release-please and tag them with a GitHub App

- Status: Accepted. Amends [0015](0015-publish-the-signed-image-to-ghcr-with-docker-buildx.md): the `v*`
  tag ruleset is split in two, and tags are created by the release App.
- Date: 2026-11-12

## Context

A release is cut by pushing a `vX.Y.Z` tag on `main`, which starts `.github/workflows/release.yml`:
GoReleaser, then the signed and attested `shift2eu` and `shift2eu-server` images (ADR 0015). Until now
the `release-tag` skill chose the version from the Conventional Commits since the last tag, waited for
the maintainer's confirmation and pushed the tag. The commits already carry everything that decides the
version; only the decision to ship should need a human (issue #142).

Constraints:

- `release.yml` must keep publishing, signing and attesting, so the signing identity
  `.../.github/workflows/release.yml@refs/tags/vX.Y.Z`, the verification commands of
  [`docs/guide/verify-releases.md`](../guide/verify-releases.md) and the `verify` job (issue #99) stay
  valid.
- A tag created with `GITHUB_TOKEN` starts no workflow, so it would start no release.
- The `v*` tag ruleset lets only maintainers create, move or delete release tags. It is the security
  boundary of the release identity (ADR 0015).
- One release at a time: GitHub keeps one pending run per concurrency group and cancels the older
  pending one, so a second tag pushed while a release run waits cancels it (ADR 0015).

## Decision

**release-please proposes every release in one release PR on `main`; merging that PR tags it, with a
GitHub App's token, and the tag starts `release.yml` as before.**

- `.github/workflows/release-please.yml` runs `googleapis/release-please-action` on every push to
  `main`. It keeps one release PR open, labelled `autorelease: pending`, whose version is computed from
  the Conventional Commits since the last release, and maintains `CHANGELOG.md` (created by the first
  release PR; it is not seeded).
- `release-please-config.json` holds the `release-tag` skill's rules: `bump-minor-pre-major: true`
  (a breaking change is minor on `v0.x`, major from `v1`), `bump-patch-for-minor-pre-major: false`
  (`feat` stays minor), `fix`/`perf` are patch, and `docs`, `test`, `ci`, `chore`, `build`, `style` and
  `refactor` are hidden changelog sections: release-please opens no release PR when the changelog it
  would write is empty, so those commits alone release nothing. Tags are exactly `vX.Y.Z`
  (`include-v-in-tag: true`, `include-component-in-tag: false`). `.release-please-manifest.json` holds
  the current version, seeded with `0.2.0`, the latest `v*` release tag on `main`.
  `check.ReleasePleaseConfig` refuses any other setting that could move the version or the tag.
- **`skip-github-release: true`.** GoReleaser in `release.yml` keeps creating the GitHub Release.
  release-please creates a tag only by creating a Release, so it creates none, and it opens no new
  release PR while a merged one is still labelled `autorelease: pending`. The workflow therefore tags
  in a step of its own. It lists the closed PRs into `main` from release-please's branch through
  `GET /repos/{owner}/{repo}/pulls?state=closed&base=main&head=<owner>:<branch>`, every page, and keeps
  those labelled `autorelease: pending`; never through the issues listing, which an App token reads
  only with Issues read. Its candidates are those merged that the App itself opened
  (`<app slug>[bot]`, from the token action's `app-slug` output); a PR closed without merging, or
  opened by anyone else, is skipped with a warning, so a label set by hand can neither release nor
  block a release. For the one candidate it reads the version from
  `.release-please-manifest.json` at the PR's merge commit, holds it to `^[0-9]+\.[0-9]+\.[0-9]+$` and
  to the PR's title, creates the annotated tag `vX.Y.Z` on that merge commit through the Git database
  API, labels the PR `autorelease: tagged` and then removes `autorelease: pending` (a run stopped
  between the two leaves both, and a re-run finds the tag on the merge commit and finishes). It fails
  closed: on several candidates, a PR not merged into `main`, a head other than release-please's
  branch, a PR that changes anything but `CHANGELOG.md` and `.release-please-manifest.json`, a merge
  commit not on `main`, or an existing tag on another commit. Pull request data reaches the shell only
  through `gh`'s output into quoted variables, never through a `${{ }}` expression, and is printed with
  `printf %q`, so it cannot emit a workflow command. `release.yml` is unchanged.
- **The release branch is `release-please--branches--main--components--shift2eu`, exactly.** The
  pinned `release-please-action` v5.0.0 bundles release-please 17.6.0. With one package in
  `release-please-config.json`, its manifest defaults `separate-pull-requests` to true
  (`src/manifest.ts`), so no merge plugin runs; the strategy then names the branch after its component,
  the package's `package-name` `shift2eu`, whatever `include-component-in-tag` says
  (`src/strategies/base.ts`, `BranchName.ofComponentTargetBranch`). `release-please--branches--main`
  is the merge plugin's branch, used only with several packages or `separate-pull-requests: false`,
  and is refused. A release-please update that changes the branch fails the tag step, closed, until
  the workflow follows it.
- **The tag is created with a GitHub App's token**, minted per run by `actions/create-github-app-token`
  and revoked when the job ends. A tag the App creates starts `release.yml`; one created with
  `GITHUB_TOKEN` would not. The App's installation has **Contents: read and write** (the release branch,
  the tag) and **Pull requests: read and write** (the release PR and its labels) only, and the workflow
  asks for exactly those two. Adding and removing labels on a pull request goes through the Issues
  labels endpoints, which accept either Issues or Pull requests write; the two `autorelease` labels are
  created beforehand by a maintainer, so the App needs no Issues permission. The tag step's calls and
  the App token's permission each needs:

  | Endpoint | Call | Permission |
  |---|---|---|
  | `pulls` (list), `pulls/<n>`, `pulls/<n>/files` | `GET` | Pull requests: read |
  | `issues/<n>/labels` (add), `issues/<n>/labels/<name>` (remove) on the PR | `POST`, `DELETE` | Pull requests: write |
  | `contents/.release-please-manifest.json`, `compare/<sha>...main` | `GET` | Contents: read |
  | `git/matching-refs/tags/<tag>`, `git/ref/tags/<tag>`, `git/tags/<sha>` | `GET` | Contents: read |
  | `git/tags`, `git/refs` | `POST` | Contents: write |

  `check.ReleasePleaseWorkflow` holds the tag step to these endpoints alone. release-please writes only
  its own branch, but `Protect main` does not stop the token from doing more: see the residual risk
  below.
- **The App's ID and key live in the `release-please` environment**, limited to `main` by its
  deployment branch policy: the `RELEASE_APP_ID` variable and the `RELEASE_APP_PRIVATE_KEY` secret. A
  workflow on another branch cannot read the key. `GITHUB_TOKEN` holds `permissions: {}` at the top
  level and `contents: read`, `actions: read`, `pull-requests: read` in the job (the guard lists the
  release PRs with the tag step's own pulls query). The job runs on `ubuntu-latest` alone, never a
  self-hosted runner. The App's token reaches two places only: release-please's `token` input and the
  tag step's `GH_TOKEN`, whose `env:` is exactly that token, the repository and the App's slug, and
  whose `gh api` calls reach only the release PR's, the manifest's, the merge commit's and the tag's
  endpoints: no Release, no `gh release`, no `curl`.
- **Concurrency.** The workflow has a group of its own, `release-please`, with
  `cancel-in-progress: false`. Sharing `release.yml`'s `release` group would let a later push to `main`
  queue a `release-please` run that cancels a pending release run. One release at a time is kept by a
  guard step before the tag: when a merged release PR of the App is still labelled
  `autorelease: pending`, it fails while `release.yml` has a run requested, waiting, pending, queued
  or in progress (`GITHUB_TOKEN`, never the App's token). With nothing to tag it passes, so a push to
  `main` during a release does not fail the workflow. The PR then stays `autorelease: pending`, and
  the next push to `main`, or a re-run of the workflow once the release finishes, tags it.
- **Two tag rulesets.** A ruleset's bypass list applies to all of its rules, so the App cannot be
  allowed to create release tags in the existing `Release tags` ruleset without also being allowed to
  move or delete them. The maintainer splits it when this change merges:

  | Ruleset | Target | Rules | Bypass (Always) |
  |---|---|---|---|
  | `Release tag creation` (new) | `refs/tags/v*` | creation | Repository admin, the release App |
  | `Release tags` (existing) | `refs/tags/v*` | update, deletion, non_fast_forward | Repository admin |

  The `release-tag` skill accepts the union of the active `v*` tag rulesets: creation, update and
  deletion must each be restricted; the release App may bypass creation only; nothing else but
  maintainers may bypass anything.
- `check.ReleasePleaseWorkflow` pins the workflow (trigger, permissions, environment, the App token's
  inputs, the runner, the token each step uses, the guard, the tagging step, its env and endpoints,
  commit-pinned actions, nothing that publishes), and CI's `Release please dry run` job runs the guard
  and tag steps' own scripts, read from the workflow, against a fake `gh` with canned API responses;
  `check.ReleasePleaseDryRun` holds that job to every pull request and push to `main`, with no write
  permission and no secret.

## Alternatives considered

- **Release on every `feat` or `fix` merge (semantic-release style).** Every merge would publish
  binaries and signed images, with no point to batch changes or review the notes. Not chosen
  (issue #142).
- **Keep the manual tag (the `release-tag` skill as it was).** The version decision is mechanical and
  already described by the commits; the skill only re-derived it by hand.
- **Let release-please create the GitHub Release (and so the tag).** GoReleaser would then find a
  Release it did not create, and the Release would exist before its assets, signatures and images.
  Keeping GoReleaser the Release's only author keeps `release.yml` and its identity unchanged.
- **Create the tag with `GITHUB_TOKEN`.** The tag would start no workflow, and only maintainers may
  create `v*` tags.
- **A personal access token.** Long-lived, tied to a person, and broader than one repository's two
  permissions. An App's installation token lives an hour at most and is revoked at the end of the job.
- **`git push` of the tag.** It needs a checkout with credentials persisted; the API call creates the
  same ref with nothing on disk.
- **Share the `release` concurrency group.** See above: a queued `release-please` run would cancel a
  pending release run.

## Consequences

- Cutting a release is merging the release PR. The `release-tag` skill now reviews the open release PR
  (version, changelog and commits against its bump rules), and after the merge verifies the tag, the
  `release.yml` run and the published images.
- A new credential: the release App's private key. It is held by the `release-please` environment,
  limited to `main`, and can create release tags (not move or delete them), push branches, manage pull
  requests and, while `Protect main` asks for no approving review, merge a pull request into `main`
  once its checks pass (see the residual risk below). It is the second identity that can start a
  release, so the App is installed on this repository only, with the two permissions above.
- **Residual risk: one App token, two holders, a tag anywhere.** The one installation token is handed
  to the third-party `googleapis/release-please-action` as well as to the tag step. With Contents
  write and the creation bypass of `Release tag creation`, whoever holds it can create a `v*` tag on
  any commit of the repository, not only on `main`, and can create or edit GitHub Releases and their
  assets. `release.yml` runs from the tagged commit's workflow file, so a tag off `main` reaches the
  signing jobs: a compromised release-please action, or a compromised step of the job, could publish a
  signed release of a commit no one reviewed. `release.yml`'s ancestry guard runs from that same
  commit, so it does not stop this. Mitigations taken: the action is pinned by commit digest and moved
  only by a reviewed Renovate PR; the tag step tags only a merged PR the App opened, from
  release-please's exact branch, changing only `CHANGELOG.md` and `.release-please-manifest.json`, on
  a merge commit on `main`; the key lives in an environment limited to `main`; the job runs only on
  GitHub's runners. Recommended to the maintainer, not done by this change (it creates no App and
  changes no setting): enable immutable releases, so a published release's assets cannot be replaced;
  split the App in two in a follow-up, a non-bypassing App for `release-please-action` and a tag-only
  App that may bypass `Release tag creation`, used only in a separate job that runs no third-party
  action; and optionally put `release.yml`'s signing jobs behind an environment with required
  reviewers.
- **Residual risk: the App can merge to `main`.** `Protect main` asks for no approving review
  (`required_approving_review_count: 0`) and has no bypass actors, so it requires a pull request and
  its status checks, nothing more. The App's token, with Contents and Pull requests write, can push a
  branch, open a PR and merge it through the API once the required checks pass, its own release PR
  included: the human merge gate holds only by convention. Recommended to the maintainer, not done by
  this change: either add a rule restricting `update` of `main` with Repository admin as the only
  bypass, in pull-request-only mode, or require at least one approving review
  (`required_approving_review_count >= 1`); check either against how agent PRs are merged today, so
  the maintainer can still merge them.
- **Residual risk: one job, shared environment files.** The tag step runs in the same job as the
  third-party release-please action. A compromised step can write `$GITHUB_ENV` or `$GITHUB_PATH` and
  plant a fake `gh` or `jq` that the later tag step runs with the App's token. Another reason for the
  recommended split into two Apps and a separate, tag-only job. In the design's favour: the App has
  no Workflows permission, so a stolen token cannot change `.github/workflows/*`. That limit is
  narrow: the token can still change the files those workflows run (`.goreleaser.yaml`, the
  `Makefile`, `.github/scripts/*`, the Dockerfiles, the Go code) on a branch it then tags or merges,
  which is what the residual risks above describe.
- Before this change merges, a maintainer must:
  - create the App with Contents and Pull requests read and write, private (installable on this
    account only), with its webhook off, and install it on this repository only;
  - create the `release-please` environment with deployment branches "Selected branches" limited to
    `main` alone, holding `RELEASE_APP_ID` and `RELEASE_APP_PRIVATE_KEY`;
  - create the labels `autorelease: pending` and `autorelease: tagged`;
  - add a branch ruleset on `release-please--branches--*` that restricts creation, update and deletion
    to the release App and repository admins, so no one else can push to the release PR's branch;
  - make CI's `Release please dry run` job a required status check of the `Protect main` ruleset.
  When it merges, split the tag ruleset as in the table. Until the App exists, the workflow fails on
  every push to `main` at the token step, and nothing is tagged.
- Dependency updates follow their commit type: a Renovate `chore(deps)` commit (the Go toolchain,
  actions, tools) releases nothing on its own, a `fix(deps)` one a patch. A dependency update that must
  ship, such as a security fix in the Go toolchain, needs a `fix` commit or a `Release-As: X.Y.Z`
  footer in a commit's body.
- A release PR whose tag could not be created (a release in flight, a failure) stays labelled
  `autorelease: pending` and blocks the next release PR until it is tagged, by the next push to `main`
  or a re-run. A PR labelled `autorelease: pending` that was closed without merging, or that the App
  did not open, is skipped with a warning on every run and never tagged; a maintainer removes its label
  to silence the warning. A triage user who re-applies `autorelease: pending` to an earlier release PR
  of the App, already tagged, makes the tag step count two candidates: it fails closed and tags
  nothing, but blocks every release until that label is removed.
- New actions to keep updated: `googleapis/release-please-action`, `actions/create-github-app-token`.
  The latter's `app-id` input is deprecated in favour of `client-id` from v3; moving to `client-id`
  needs the App's client ID in the environment and a matching change to `check.ReleasePleaseWorkflow`.
