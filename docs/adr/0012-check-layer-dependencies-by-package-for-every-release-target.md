# 0012. Check the layers' dependencies by package for every release target

- Status: Accepted
- Date: 2026-11-11

## Context

The dependency rule (ADR 0001, `docs/architecture.md`) is enforced by depguard in `.golangci.yml`.
depguard reads the import declarations of the Go files golangci-lint analyses and picks the rule for a
file by its file name. The security audit of #33 found code that reaches I/O from the domain with no
lint error (issue #40):

- **Assembly.** A bodyless Go declaration in a domain package and a `zz_arm64.s` that makes a raw
  `write` system call. The Go file imports nothing, and depguard never reads the `.s` file.
- **Line directive.** A domain file that starts with `/*line ../../version/version.go:1*/` and imports
  `os`. depguard matches its file globs against the position the directive names, so `domain-is-pure`
  never applies.
- **Symlinked package directory, a package under `_` or `testdata`, a nested module.** #33 refuses an
  import of these by naming the domain packages one by one. A reviewed change that adds one to an
  allow list brings the gap back: `./...` never lints such a package's own files, and a symlinked one
  is linted under its real path.

A file-name-based rule cannot close these: the file name is what each of them changes. What the build
compiles is known per package, from the go command. Issue #39 adds that the platform matters too: a
file behind `//go:build windows` or `_arm64` is compiled only for that platform, and lint runs for the
host's.

Forces:

- **One source of truth.** The release platforms are in `.goreleaser.yaml` and the allow lists in
  `.golangci.yml`. A copy of either in a check would drift.
- **No new dependency** (CLAUDE.md rule 6). The standard library has no YAML reader.
- **Default tests stay offline and need no credentials**, and the check must run where lint runs:
  `make check` and CI.
- **Fail closed.** A check that silently reads less than it should is worse than none.

## Decision

A second, package-level enforcement of the dependency rule lives in the test package
`test/architecture`, beside depguard; its code is in `test/architecture/internal/check`, which no
other package can import. For every platform the release builds, with cgo off and on, and with no
build tag, `e2e`, `integration` and both, it runs `go list -deps` twice:

- **The layers with a strict depguard rule**: every directory the `files:` list of a strict rule
  names (`check.RuleDirs`), today the domain, the application layer, the target snapshot reader, the
  adapter helpers under `internal/adapter/internal` (`jsondoc` among them), the inventory file reader
  and the OVHcloud and Scaleway targets. The directories are read from `.golangci.yml`, not copied,
  and a rule must write each as `**/<dir>/**` and take test files out with `!$test`; any other shape
  is an error. Every package of the module it lists is held to the allow list of every rule whose
  directory covers it, as depguard holds a file to every rule whose globs match it: it must belong
  to the main module, sit in the directory its import path names once symlinks are resolved, hold no
  cgo, C, assembly, syso or other non-Go source, and import only what its rule allows
  (`check.Packages`). Any other non-standard package the layers reach is flagged.
- **The release binary**, `./cmd/eusov`: every package under the module path must belong to the main
  module, sit in its own real directory and hold only Go sources (`check.Sources`). Imports are not
  checked there; third-party packages are skipped.

Every strict rule in `.golangci.yml` must be a layer of that check or a skip with a written reason
(`TestEveryStrictRuleIsChecked`): `cli-gets-adapters-from-cmd` allows `$gostd`, which no package list
can spell out, and `e2e-is-black-box` holds test files, which `go list -deps` does not list. A further
test refuses a line directive (`//line ` or `/*line `, found with `go/scanner`) in any Go file of the
repository, testdata and `_` directories included, and any symlink. The files are those
`git ls-files --cached --others --exclude-standard` lists: everything tracked, and everything
untracked that git does not ignore, so a local `bin/`, `dist/` or editor directory is not read. When
git cannot list the files the test fails; it is not skipped.

Both lists are read when the tests run: the platforms from `.goreleaser.yaml`, the allow lists and
strict rules from `.golangci.yml`. Both files are read with the standard library, by a line reader
keyed on indentation that follows the path to the value it needs and refuses what it does not expect
(anchors, aliases, merge keys, quoted or upper-case keys, escapes, a repeated key). Anywhere in either
file, comments included, it refuses invalid UTF-8, a tab, a CR not followed by LF, any other control
character (NEL among them), U+2028, U+2029 and U+FEFF: YAML readers take some of them for line
breaks where this reader would not, so a list entry could be hidden from the check and not from the
tool. CRLF line ends and other printable text are accepted.

The check fails closed:

- `.goreleaser.yaml` may hold only `version`, `project_name`, `before`, `builds`, `archives`,
  `checksum` and `changelog` at the top level (a top-level `env:` reaches every build, and
  `gomod.gobinary` picks the toolchain). `before` may hold only `hooks`, exactly `[go mod tidy]`. There
  must be exactly one build, holding only `id`, `main`, `binary`, `env`, `flags`, `ldflags`, `goos` and
  `goarch`; `main` is `./cmd/eusov`, `env` exactly `CGO_ENABLED=0` (goreleaser renders each entry,
  name included, as a template), `flags` only `-trimpath`, and no template in `goos`, `goarch` or
  `flags`. No other goreleaser config file may exist and the release workflow passes goreleaser exactly
  `release --clean`,
  so the file the check reads is the file the release builds from.
- No `go.work`, `go.work.sum` or `vendor` that git could commit may exist at the repository root
  (`check.StrayReleaseFiles`): none that `git ls-files --cached --others --exclude-standard` lists,
  so a tracked or force-added one, or an untracked one git does not ignore. The check lists with
  `GOWORK=off` and `-mod=readonly`, so none of them changes what it lists, but the go command
  goreleaser runs reads all three: without this, a committed one would change what the release builds
  and not what the check reads. A developer's own `go.work`, ignored by `.gitignore`, never reaches
  the release, which builds from a fresh checkout, so it is allowed. When git cannot run, the check
  fails.
- `.github/workflows/release.yml` is read as a closed schema (`check.ReleaseWorkflow`). Its top-level
  keys are exactly `name`, `on` (a push of `v*.*.*` tags), `permissions` (`{}`) and `jobs`; its jobs
  exactly `test` and `goreleaser`, each holding exactly `name`, `runs-on`, `permissions` and `steps`,
  and `goreleaser` also `needs: test`. The `test` job, with `contents: read`, runs exactly
  `actions/checkout`, `actions/setup-go` and `make test`. The `goreleaser` job, the only one with
  `contents: write`, runs exactly `actions/checkout`, `actions/setup-go` and
  `goreleaser/goreleaser-action`, in that order, each action pinned by commit and with exactly the
  `with:` keys and values listed: goreleaser at an exact version, with `args: release --clean`, and an
  `env:` of exactly `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`. Any other key, job, step, `run:` or
  `uses:` is refused. The tests run repository code, so they run in a job of their own: a job starts
  on a fresh runner, so neither a go env file (`go env -w`) nor `GITHUB_ENV` or `GITHUB_PATH` carries
  over from them to goreleaser. A fresh runner is not enough on its own: setup-go's Actions cache restores
  `GOMODCACHE` and `GOCACHE` under a key of runner OS, Go version and `go.sum` hash, shared with the
  `test` job and with CI on main, and the go command trusts `GOCACHE` entries and does not verify
  extracted modules again. So `actions/setup-go` must have `cache: false` in both jobs. As a backstop, no line, comments included, may name `github.env` or
  `github.path`, `GITHUB_ENV` or `GITHUB_PATH`, or a variable of the go command or of cgo (a word in
  upper case starting `GO` or `CGO`: `GOFLAGS`, `GOEXPERIMENT`, `GOAMD64`, `CGO_ENABLED`, ...).
- A depguard rule it reads must be `list-mode: strict` with a non-empty allow list whose every entry
  ends in `$`, so the list names packages exactly.
- `go list` runs with an environment built from an allow list of inherited variables (`PATH`,
  `GOROOT`, `HOME`, `USERPROFILE`, `XDG_CACHE_HOME`, `LOCALAPPDATA`, `GOPATH`, `GOMODCACHE`, `GOCACHE`,
  `GOTOOLCHAIN`, `TMPDIR`, `TEMP`, `TMP`) plus `GOENV=off` (an empty variable does not override a
  `go env -w` value), `GOFLAGS=-mod=readonly`, `GOPROXY=off`, `GOWORK=off`, `GOOS`, `GOARCH` and
  `CGO_ENABLED`, so neither the developer's environment nor their go env file can add a tag, an
  experiment or a direct fetch. It prints only the fields the check decodes. A package that `go list`
  reports an error for, or lists as incomplete, fails the check; a directory it cannot resolve is a
  violation.

The checks are ordinary tests, so `make test`, `make check` and CI's `make cover` run them with no
workflow change.

## Alternatives considered

- **A CI grep for `//line` and `/*line`, `.s` files and symlinks** (the issue's first proposal for line
  directives). A grep reads text, not Go: it flags a directive inside a string literal and needs its
  own list of file kinds and directories to skip. `go/scanner` reads comments as the compiler does,
  and `go list` reports every source file kind the build uses.
- **More depguard rules or golangci-lint settings.** Any rule depguard can hold is selected by file
  name, which is the input each probe controls.
- **A custom analyzer in golangci-lint.** It would still see only the files golangci-lint loads, for
  the host's platform, and would need a plugin build of golangci-lint.
- **Hard-coding the platforms and allow lists in the test.** A second copy that drifts from
  `.goreleaser.yaml` and `.golangci.yml`, which the review of either file would not see.
- **A YAML library (`gopkg.in/yaml.v3` or similar).** It would read both files fully, but it is a new
  dependency for a test, and a general reader accepts anchors, merge keys and the rest, which the
  strict reader refuses so that a change to either file cannot quietly shrink what the check reads.

## Consequences

- Assembly, cgo, line directives, symlinked directories, `_` and `testdata` packages and nested modules
  can no longer get I/O into a layer with a strict rule without a failing test, on any release
  platform, and no package of the module in the release binary can hold non-Go sources or sit
  outside its own directory.
- A change to a strict rule in `.golangci.yml` is checked package by package too, a new strict rule
  fails `TestEveryStrictRuleIsChecked` until it has a layer or a reasoned skip, and a change to the
  platform list in `.goreleaser.yaml` is picked up without editing the check.
- A new key, hook, env entry or flag in `.goreleaser.yaml`, or a YAML feature or character the reader
  does not follow, fails the test until `test/architecture/internal/check` is taught it. That is the
  point: whoever changes how the release is built decides whether the check still holds.
- What stays outside the check:
  - **The runner and the actions the release workflow uses.** The goreleaser job runs no repository
    code before goreleaser, and its steps are a closed list of pinned actions, so the variables of the
    go command can come only from the runner image or from inside a pinned action
    (`actions/checkout`, `actions/setup-go`, `goreleaser/goreleaser-action`). A repository or
    organisation variable reaches a step only through an expression in the workflow file, which the
    schema refuses everywhere but the values it pins. Review of the action updates holds the rest.
  - **Third-party modules.** Their own sources are not checked; only the imports of the module's
    packages that import them, where a strict rule exists. The release binary has none today.
  - **I/O through functions of allowed packages** (`fmt.Print*`, `time.Now`, ...), which no import
    rule sees. Review holds that line, as the comment on `domain-is-pure` says.
  - **The imports of layers without a strict rule** (the CLI, the target and report adapters,
    `cmd/eusov`): the release-binary pass checks their sources and directories, not their imports.
- `make test` runs `go list` 96 times (6 platforms, cgo off and on, 4 tag sets, layers and binary).
  They run in parallel and take a few seconds.
- The rules themselves are pinned too (issue #80): `TestDepguardRulesArePinned` reads every depguard
  rule of `.golangci.yml` whole (list-mode, files, allow, the `pkg` of each deny entry, a shared deny
  list read through its YAML anchor) and compares it with `pinnedDepguardRules`. A rule dropped or
  weakened by a merge fails the test with the rule named; a deliberate change updates the pin in the
  same commit, where review sees it. `TestDepguardPinCatchesMutations` proves the pin is not vacuous
  with 532 mutations of the real file: each rule and each files, allow and deny entry deleted, the
  trailing `$` of each exact allow entry dropped, an allow entry `os` and a negated files glob added,
  and each list-mode rewritten, deleted or added. A self-check fails if it misses an entry, and a
  refused document counts as caught only where deleting `app-uses-ports-not-adapters` removes the
  `&app-deny` anchor.
- So is whether they run (issue #81): `TestGolangciSettingsHideNoIssue` reads `.golangci.yml` with the
  same reader and fails, naming the setting, on every setting that hides issues without touching a
  rule: exclusion paths, a `linters.exclusions.generated` other than `disable` (absent means `lax`),
  `issues` caps and filters, `run.issues-exit-code: 0`, `linters.default` other than `standard` or
  `all`, `run.tests` not true, depguard or forbidigo not enabled or disabled, and an exclusion rule
  naming depguard, or forbidigo without a text or with an empty one. The document's shape is an allow
  list that fails closed: `version` must be `"2"`, and a key or preset it does not know is refused.
  `TestRepositoryReadsOnlyGolangciYML` refuses a `.golangci.json`, `.golangci.toml` or
  `.golangci.yaml` that git could commit, which golangci-lint would read before `.golangci.yml`; the
  lint command lines, in the Makefile and the CI step, are pinned already by `test/architecture`.
- Lint and this check now say the same thing twice: a violation that lint catches also fails here.
