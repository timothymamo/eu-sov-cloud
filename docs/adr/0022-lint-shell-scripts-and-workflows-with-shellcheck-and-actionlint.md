# 0022. Lint shell scripts with shellcheck and workflows with actionlint

- Status: Accepted
- Date: 2026-11-12

## Context

The repository holds shell scripts that the release, CI and the developer environment run:
`.github/scripts/verify-archives.sh` and `.github/scripts/release-please-dry-run.sh`, the scripts
under `dev/`, and the git hooks under `.githooks/`. The workflows under `.github/workflows/` also hold
long inline `run:` scripts, the riskiest of them in `release.yml`, which signs and publishes. Nothing
lints any of it. A quoting bug, an unset variable or a word-splitting mistake shows up only in review
or in a failed release, and so does a wrong `${{ }}` expression or a context a job cannot read
(issue #164).

Constraints:

- Every tool version is pinned and moved by Renovate (CLAUDE.md rule 7), and a new third-party
  dependency needs a reason (rule 6).
- `make lint` and CI's required `Lint` check must report the same findings, and the Lima VM must have
  every tool `make lint` runs.
- `make lint` tries every step and fails at the end, so one run reports everything.
- Go code may not silence a linter without naming it and giving a reason (nolintlint, `test/lint`).
  Shell scripts need the same rule.

## Decision

**`make lint` runs shellcheck on every shell script git tracks and actionlint on the workflows. CI runs
both in a `lint-shell` job that the `Lint` check needs. A shellcheck disable directive must name its
codes and give a reason.**

- **shellcheck** comes from a pinned release of `koalaman/shellcheck`, verified by SHA-256 per
  architecture. It is not a Go program, so the go command cannot pin it. `dev/lima.yaml` pins the
  version and the amd64 and arm64 digests as `param:` values, with `# renovate:` comments
  (`github-releases` for the version, `github-release-attachments` with `digestVersion` for each
  digest). It installs the binary root-owned in `/usr/local/bin` through `check_pinned_sha256`, as it
  does gh. CI's `lint-shell` job pins the same version and the amd64 digest
  (`SHELLCHECK_VERSION`, `SHELLCHECK_SHA256`), installs that binary and puts it first on the PATH.
  `test/architecture`'s `TestShellcheckPinsAgree` holds the two pins equal, and one Renovate group
  moves them together.
- **actionlint** is a `tool` directive in `go.mod`, run as `go tool actionlint`. go.sum and the Go
  checksum database verify it, the gomod manager already bumps it, and it needs no download step in CI
  or in the VM: any Go install runs it. actionlint hands every `run:` script to the shellcheck on the
  PATH, so CI and the VM check workflow scripts with the pinned shellcheck too.
- **What shellcheck checks:** `dev/shellcheck.sh` takes the list from `git ls-files`, never from a
  hand-written list. A script is a tracked file whose name ends in `.sh`, or whose first line is a
  shebang that runs `sh` or `bash`, directly or through `env`. That is the rule `test/lint`'s
  `IsShellScript` states, and `TestMakeLintRunsEveryReleaseTarget` fails when the two pick different
  files. A new script is checked without any edit.
- **Severity: `--severity=style`**, every finding, which is shellcheck's default, stated so it cannot
  drift. The word splitting the issue names, SC2086, is only `info`, as is SC2016. A
  `--severity=warning` run would let through exactly the class of bug that motivated this. With every
  finding fixed, nothing in the repository needed a lower bar.
- **Findings are fixed, not silenced.** Where a check cannot be satisfied, the line carries
  `# shellcheck disable=SCnnnn # reason`. `test/lint`'s `TestTrackedShellScriptsNameTheirShellcheckCodes`
  refuses a directive without a code of the form `SC` and four digits (`all` included) or without a
  reason on the same line, as nolintlint does for `//nolint`.
- **Where they run:** once in `make lint`, after the golangci-lint runs and before `test/lint`, through
  the same `failed=` accounting. Neither depends on the release target, so neither runs once per
  target. In CI they run in a job of their own, not as steps of the `lint-target` matrix, which would
  run them seven times. The aggregate `Lint` job needs both jobs, so the required check still gates
  them.

## Alternatives considered

- **`--severity=warning`.** Fewer findings to fix, but SC2086 (unquoted expansions split into words)
  is `info`. That would leave out the bug class the issue was opened for.
- **The runner's preinstalled shellcheck.** It is unpinned and changes when GitHub updates the image,
  so CI and the VM would drift, and a runner update could fail every PR at once.
- **A pinned shellcheck action, or `pip`/`apt` packages.** An action adds a second pin of the same
  tool that cannot share the VM's. The packages lag behind releases and differ between Ubuntu
  versions.
- **actionlint from a pinned release with SHA-256 digests,** like shellcheck. That needs a download
  step and digests in two places. The `go tool` directive is verified by go.sum and needs neither. Its
  cost is twelve indirect requirements in `go.mod` (actionlint and its dependencies, such as
  `fatih/color`, `robfig/cron` and `golang.org/x/sys`). No package of the module imports them, so the
  release binaries, depguard and govulncheck's scan of `./...` never see them.
- **No actionlint.** shellcheck alone would leave the `run:` scripts, the bulk of the release logic,
  unlinted, and nothing would check `${{ }}` expressions and contexts.

## Consequences

- A shell script or a workflow change that shellcheck or actionlint refuses fails `make lint`, so
  `make check`, and the required `Lint` check.
- Running `make lint` locally now needs shellcheck on the PATH: `docs/contributing/development.md`
  says how to install the pinned version. actionlint needs nothing beyond Go.
- A shellcheck bump arrives as one Renovate PR for `dev/lima.yaml`, its `lima-invalid` copies and
  `ci.yml`, and it fixes any new finding in the same PR. shellcheck releases carry no signature or
  attestation, so a digest can be checked only against the asset GitHub serves. CI's `lint-shell` job
  does that on every run.
- The Lima VM's own provisioning scripts in `dev/lima.yaml` are YAML strings, not tracked scripts, so
  they are not linted. Extracting them would be a separate change.
