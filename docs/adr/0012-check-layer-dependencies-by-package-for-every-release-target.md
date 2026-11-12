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
other package can import. (Issue #82 moved the release-target reader to
`test/internal/releasetarget` and the YAML line reader to `test/internal/yamlread`, so `test/lint`
reads the same list with the same reader; `test` alone can import them.) For every platform the
release builds, with cgo off and on, and with no build tag, `e2e`, `integration` and both, it runs
`go list -deps` twice:

- **The layers with a strict depguard rule**: every directory the `files:` list of a strict rule
  names (`check.RuleDirs`), today the domain, the application layer, the target snapshot reader, the
  adapter helpers under `internal/adapter/internal` (`jsondoc` among them), the inventory file reader
  and the OVHcloud and Scaleway targets. The directories are read from `.golangci.yml`, not copied,
  and a rule must write each as `**/<dir>/**`; it may take whole directories under them out again
  with `!**/<dir>/**`, and test files out with `!$test`; any other shape is an error. Every package
  of the module it lists is held to the allow list of every rule whose directory covers it and does
  not exclude it, as depguard holds a file to every rule whose globs match it: it must belong to the
  main module, sit in the directory its import path names once symlinks are resolved, hold no cgo,
  C, assembly, syso or other non-Go source, and import only what its rule allows
  (`check.Packages`). An allow entry ending in `$` allows that package alone; one ending in `/` is
  a prefix, as depguard reads it, and only the entry sorted nearest before an import decides. A
  rule without `!$test` holds the imports of the package's `_test.go` files too (go list's
  `TestImports` and `XTestImports`, integration files in the `integration` passes). A third-party
  package an allowed import reaches, and its own dependencies, are exempt; any other non-standard
  package the layers reach is flagged.
- **The release binary**, `./cmd/shift2eu`: every package under the module path must belong to the main
  module, sit in its own real directory and hold only Go sources (`check.Sources`). Imports are not
  checked there; third-party packages are skipped.

Every strict rule in `.golangci.yml` must be a layer of that check or a skip with a written reason
(`TestEveryStrictRuleIsChecked`): `cli-gets-adapters-from-cmd` allows `$gostd`, which no package list
can spell out, `e2e-is-black-box` holds a directory of test files only, which `go list -deps`
does not list as a layer (`app-tests-use-fakes-not-adapters`, which holds the test files of a layer's
packages, is checked: *Amended (issue #144)*, below), and `composition-root` is held on its own (*Amended (issue #101)*, below).

*Amended (issue #79):* the check first required every allow entry to end in `$` and every rule to
name `!$test`, so it skipped `cloud-adapters-reach-the-cloud-through-the-sdk`, whose allow list
names the cloud SDK roots as prefixes and whose `files:` list takes `inventoryfile` and
`sourcetest` out of `internal/adapter/source` and holds test files. It now reads prefix entries,
negated whole-directory globs and test-file imports, and holds that rule like every other. It
matches an import as depguard v2.2.1 does: the allow entries are sorted as written, `$` included,
and only the entry nearest before the import decides, so `a/b$` shadows `a/` for `a/c`. A prefix
entry must end in `/` (`os` would also allow `osfoo`), and a rule whose deny list names a path its
allow list allows is refused, since depguard then decides by entry length; the deny list is read
fail-closed. A third-party package is exempt from the check, sources included, when every in-module
package that imports it directly is held by a strict rule and every rule holding that import allows
it; so is every package it imports that no in-module package imports directly. depguard lints no
file of a dependency, and a real SDK may hold assembly. A third-party package that a package no
strict rule holds (one outside every directory, or one a rule excludes) imports directly is
reported, as is one no allow list reaches. The exemption trusts the import path, which is sound only
because nothing can substitute other code for a path a prefix entry allows: the root `go.mod` holds
no `replace` directive, read with the go command's own parser (`go mod edit -json`,
`TestRootModuleHasNoReplace`), and the repository holds no `go.work`, `go.work.sum` or `vendor/`
directory (`TestRepositoryHasNoWorkspaceOrVendor`), each of which could otherwise let a fork stand in
for an allowed import path. It reads the strict allow lists only:
the lax deny rules, such as `go-cmp-only-in-tests` and `cloud-sdks-only-in-provider-adapters`, are
held by depguard in `make lint` alone, so a non-test go-cmp import or an SDK import in a new helper
beside the provider adapters passes this check and fails lint. The layers do not include
`cmd/shift2eu`, `internal/cli`'s rule allows `$gostd` and is skipped, and `check.Sources` skips every
package outside the module, so the layer listing holds no third-party import in either. A
third-party import in `internal/cli` is refused by depguard alone: its strict rule
`cli-gets-adapters-from-cmd` allows only `$gostd` and named module packages. One in `cmd/shift2eu` was
then refused by no rule but the lax SDK and go-cmp denies, a gap left to a follow-up issue and closed
by the amendment below.

*Amended (issue #101):* the composition root has a strict depguard rule of its own,
`composition-root`. Its `files:` list is `**/cmd/shift2eu/**` and `!$test`; its allow list is the module
path, `github.com/timothymamo/shift2eu/`, as its one prefix entry, so `cmd/shift2eu` may import every
package of the module, and the standard packages it imports today (`context`, `os`, `os/signal`,
`syscall`), each exact; it denies the four cloud SDK roots as well. Any other import, a logging
library or an HTTP client, fails `make lint`. The rule is a reasoned skip of the shared layer listing,
since `go list -deps ./cmd/shift2eu/...` also lists `internal/cli` and `internal/version`, which no rule
the check reads holds. Instead `TestCompositionRootIsHeldToItsRuleForEveryReleaseTarget` lists
`./cmd/shift2eu/...` on every release target, with cgo off and on and each of the four tag sets the layer
listing reads (the `cgoModes` and `tagSets` tables both tests share), and passes `check.Packages` the
rule and the packages under `cmd/shift2eu` alone. It also fails any package of that listing under the
module path that does not belong to the main module: the rule's prefix entry would allow a module
nested under that path (the security audit's LOW-1).
`TestCompositionRootRuleHoldsItsProbeForEveryReleaseTarget` fails a fixture `cmd/shift2eu` that imports
a third-party library, a stand-in SDK and a nested module under the fixture's own module path, which
the main-module assertion must report. The rule holds
`cmd/shift2eu` only: a glob segment matches no `/`, so a sibling such as `cmd/shift2eu-server` (PRD #103) is
not covered and gets a sibling rule written the same way, with its own `files:` entry.
`TestOnlyNamedRulesHoldPrefixEntries` allows prefix entries in this rule and the SDK-only rule alone.
The rule allows `os` and `syscall`, which can start a process or read the environment, so two
forbidigo groups hold the composition roots `cmd/shift2eu` and `cmd/shift2eu-server` at the call level
(issue #111, narrowing the security audit's LOW-2): `composition-root-no-exec` refuses
`os.StartProcess`, `syscall.Exec`, `syscall.ForkExec`, `syscall.StartProcess`, any use of `os/exec`,
`os.Environ` and `syscall.Environ`, the windows entry points (`syscall.CreateProcess`,
`CreateProcessAsUser`, `GetEnvironmentStrings`, `LoadDLL`, `MustLoadDLL`, `NewLazyDLL`,
`LoadLibrary`, `GetProcAddress`), the raw system calls (`syscall.Syscall`,
`Syscall6/9/12/15/18`, `SyscallN`, `RawSyscall`, `RawSyscall6`) and the `syscall.SYS_*` numbers, and
any use of `unsafe`, in the roots' `_test.go` files too, with no exception; `composition-root-env`
refuses `os.Getenv`, `os.LookupEnv`, `os.ExpandEnv`, `syscall.Getenv` and
`syscall.GetEnvironmentVariable`, except one `os.LookupEnv(tokenEnvVar)` call in
`cmd/shift2eu-server/main.go`, in the `func() (string, bool)` token source it hands to `run` for
`SHIFT2EU_SERVER_TOKEN` (ADR 0019); `run`, `setUp` and `readToken` see only the token. Each
group has one `path-except: ^cmd/shift2eu(?:-server)?/` exclusion rule, so it holds for both roots,
`_test.go` files included, and the exception is a `path` rule for that one file whose text names the
`os.LookupEnv` entry's msg alone. The named `syscall` and `unsafe` entries are defence in depth (a
deny list of names leaked, and `fmt` and `strconv` can produce an address without `unsafe`). Four
source checks in `test/lint` add what forbidigo cannot state:
`TestCompositionRootsUseSyscallOnlyForSignals`, the comprehensive guard (the roots use `syscall` only
for a `SIG*` constant and `syscall.Signal`), `TestCompositionRootsLinkNoForeignSymbol` (no
`//go:linkname` in the roots), `TestCompositionRootPassesLookupEnvToRun` (`os.LookupEnv` in
`cmd/shift2eu-server/main.go` only as the call `os.LookupEnv(tokenEnvVar)` that the token source
passed to `run` returns) and `TestCompositionRootsNameNoEnvironmentPath` (no `/proc/` or `/dev/fd/`
literal in the roots). The roots' `_test.go` files, which the strict composition-root rules skip, are
held by the lax depguard rule `composition-root-tests` (no `go/build`, `net/http/cgi`, `os/exec`,
`plugin`, `runtime/cgo`, `syscall/js` or `unsafe`). The gap is narrowed, not closed: a root can delegate an environment read to
`internal/cli`, which may import `os`; a `/proc` path built at run time passes the literal check; and
`net/http`'s `DefaultTransport` reads the proxy variables implicitly, which is accepted
(docs/architecture.md, Composition-root process and environment rule, Known gaps).
The check anchors a rule's `**/<dir>/**` and `!**/<dir>/**` at the module root, while depguard
matches them at any depth of the file path, so a nested path such as
`internal/adapter/target/x/internal/adapter/source/y` is read differently; an exclusion must lie
under one of its rule's directories. A package that only a test imports is not itself listed by
`go list -deps`, so its own placement (module, directory, sources) is not checked; the importing
test's allow list is. A further
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
  `gomod.gobinary` picks the toolchain). `before` may hold only `hooks`, exactly
  `[go mod tidy -diff]` (*Amended (issue #102)*, below). There must be exactly one build, holding
  only `id`, `main`, `binary`, `env`, `flags`, `ldflags`, `goos` and `goarch`; `main` is
  `./cmd/shift2eu`, `env` exactly `CGO_ENABLED=0`, `GOWORK=off` and `GOFLAGS=-mod=readonly`, in that
  order (`releasetarget.Env`; *Amended (issue #102)*, below; goreleaser renders each entry, name
  included, as a template), `flags` only `-trimpath`, and no template in `goos`, `goarch` or `flags`.
  No other goreleaser config file may exist and the release workflow passes goreleaser exactly
  `release --clean`, so the file the check reads is the file the release builds from.
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

*Amended (issue #102):* the build's `env` was first exactly `CGO_ENABLED=0`. A release build now also
sets `GOWORK=off`, so a `go.work` file (in the checkout or a parent directory) cannot put the build in
workspace mode and swap in other module versions, and `GOFLAGS=-mod=readonly`, so the build fails
rather than changes `go.mod` or `go.sum`, and uses no `vendor` directory. `check.StrayReleaseFiles`
keeps refusing those files in the repository; the env makes the go command goreleaser runs ignore or
refuse them as well, so the release builds what the check lists, which already runs with `GOWORK=off`
and `-mod=readonly`. `releasetarget.Env` returns the three entries and `releasetarget.Read` holds each
build entry to exactly that list, so a second build (PRD #103's `cmd/shift2eu-server`) will share the
rule rather than copy it. The `Dockerfile`'s `go build` sets the same three, and `test/container`
fails when it or `.goreleaser.yaml` drops one (ADR 0013). The `before` hook was first exactly
`go mod tidy`; it is now exactly `go mod tidy -diff`. A hook runs before the build and without its
`env`, so plain `go mod tidy` would rewrite an untidy `go.mod` or `go.sum` (possibly with a newer
module version from `GOPROXY`) and the `-mod=readonly` build would then pass against the rewritten
files; `-diff` fails the release instead and writes nothing.

*Amended (issue #119):* the check reads the strict allow lists only, so the lax rule
`adapters-are-built-in-cmd` (files `$all`) was held by depguard alone, and depguard reported nothing
for it: its deny list held `internal/adapter/` and `internal/adapter/internal/`, and since depguard
v2.2.1 compares an import only with the entry nearest before it, every adapter under `report/`,
`request/`, `source/` or `target/` was compared with `internal/adapter/internal/` alone. Two checks
now close that. `TestDepguardEntriesShadowNothing` reads every rule with `check.DepguardRules`, the
reader of the depguard pin, aliased deny lists included, and fails on any allow or deny entry
without `$` that is a strict prefix of another entry in the same list, the shape that makes depguard
ignore the shorter one; a bare `net` or `os` beside `net/http` or `os/exec` is written `net$` or
`os$` instead, with each subpackage named. `TestAdaptersBuiltInCmdNamesEveryAdapterSubtree` fails on
a directory under `internal/adapter` that has no deny entry of its own in `adapters-are-built-in-cmd`
(both `<dir>/` and, but for `internal/`, the exact `<dir>$` for a package at the subtree's root),
or a file placed there directly, so a new adapter subtree cannot escape the rule. `test/lint`'s
`TestAdaptersBuiltInCmdProbe` runs depguard alone, through the forbidigo probe harness, on a probe
module that imports an adapter from each subtree, and each subtree's root, outside the composition roots, and runs with
`SHIFT2EU_LINT_PROBES=require` in `make lint` and every lint-target matrix entry.

The lax rule `app-tests-use-fakes-not-adapters` shares the `&app-deny` list of the strict
`app-uses-ports-not-adapters` and allows any package that list does not name, so the application
layer's tests could import a standard package that does I/O without importing `os` or `net`. The list
now also names `io/ioutil`, `log/syslog`, `text/template`, `html/template`, `archive/zip`, every
`debug/*` package, `go/build`, `go/importer`, `go/parser`, `mime/multipart`, `runtime/coverage`,
`runtime/debug` and `unsafe`, besides `syscall` and `plugin`; it
names the packages known to do I/O and does not claim to be complete. `test/lint`'s
`TestAppTestsUseFakesNotAdaptersProbe` pins it: an application-layer `_test.go` file in a probe module
imports each of them, which must be refused, and pure controls (`net/url`, `net/netip`, `strings`,
`context`, `testing`, `runtime`), which must not. `path/filepath` stays allowed on purpose: `Join` and
`Base` are pure, and nothing in `internal/app` imports it.

*Amended (issue #144):* a deny list blocks only what someone thought to name, so
`app-tests-use-fakes-not-adapters` is now a strict allow list, like the production layers' rules: the
application package under test, the six domain packages, `internal/testutil`, `go-cmp`'s `cmp`,
`testing`, the domain's pure standard packages (the allow list of `domain-is-pure`, repeated because
YAML cannot join two lists, so the domain and its caller's tests agree on what pure means) and
`bytes`, `context`, `io` and `sync`, every entry exact. Anything else fails lint, a standard package
added to Go later included; the shared `*app-deny` list stays and only says why. The rule's files are
the pair `**/internal/app/**/*_test.go` and `**/internal/app/*_test.go`, the layer's test files alone,
so it is no longer a lax rule held by depguard alone: `check.RuleDirs` reads that pair as the test
files of `internal/app` (`TestsOnly`), and `check.Packages` holds the `TestImports` and
`XTestImports` of every package under the directory to the rule's allow list, and none of their own
imports, which `app-uses-ports-not-adapters` holds, package by package on every release target, cgo
mode and tag set, as `TestReleaseTargetDependencies` holds the other strict rules. `RuleDirs` refuses
a list that names one glob of the pair without the other, mixes the pair with a directory, an
exclusion or `!$test`, and `check.Packages` flags a package held by test-file rules alone, since then
no rule holds its production imports. `check.AllowList` now follows a deny list written as an alias
(`deny: *app-deny`), as `check.DepguardRules` does, so the shadowing check between the allow and deny
lists covers the shared list too. `test/lint`'s `TestAppTestsUseFakesNotAdaptersProbe` pins both
halves: every allowed package must pass, and the I/O packages above, `encoding/xml` and `expvar` (on
no list) and `runtime` and `go/build/constraint` (pure but unneeded) must be refused.

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
  does not follow, fails the test until `test/internal/releasetarget` or `test/internal/yamlread` is taught it. That is the
  point: whoever changes how the release is built decides whether the check still holds.
- What stays outside the check:
  - **The runner and the actions the release workflow uses.** The goreleaser job runs no repository
    code before goreleaser, and its steps are a closed list of pinned actions, so the variables of the
    go command can come only from the runner image or from inside a pinned action
    (`actions/checkout`, `actions/setup-go`, `goreleaser/goreleaser-action`). A repository or
    organisation variable reaches a step only through an expression in the workflow file, which the
    schema refuses everywhere but the values it pins. Review of the action updates holds the rest.
  - **Third-party modules.** Their own sources are not checked; only the imports of the module's
    packages that import them, where a strict rule exists. The release binary has none today, and
    `composition-root` (issue #101) refuses one in `cmd/shift2eu`.
  - **I/O through functions of allowed packages** (`fmt.Print*`, `time.Now`, ...), which no import
    rule sees. Review holds that line, as the comment on `domain-is-pure` says.
  - **The imports of layers without a checked strict rule** (the CLI, whose rule allows `$gostd`,
    and the target and report adapters): the release-binary pass checks their sources and
    directories, not their imports. `cmd/shift2eu` is no longer among them: its `composition-root` rule
    is checked on its own (issue #101).
- The repository tests in `test/architecture` run `go list` 162 times: 96 for the layers and the
  binary (6 platforms, cgo off and on, 4 tag sets, one listing each), 48 for the composition root
  (the same 6 platforms, cgo modes and tag sets, issue #101), and 18 for the three probes (the
  SDK-only rule's two and the composition root's, once per platform, cgo off, no tags).
  `internal/check`'s own tests list a few fixture modules on top. They run in parallel and take a
  few seconds.
- The rules themselves are pinned too (issue #80): `TestDepguardRulesArePinned` reads every depguard
  rule of `.golangci.yml` whole (list-mode, files, allow, the `pkg` of each deny entry, a shared deny
  list read through its YAML anchor) and compares it with `pinnedDepguardRules`. A rule dropped or
  weakened by a merge fails the test with the rule named; a deliberate change updates the pin in the
  same commit, where review sees it. `TestDepguardPinCatchesMutations` proves the pin is not vacuous
  with 533 mutations of the real file: each rule and each files, allow and deny entry deleted, the
  trailing `$` of each exact allow entry dropped, an allow entry `os` and a negated files glob added,
  and each list-mode rewritten, deleted or added. A self-check fails if it misses an entry, and a
  refused document counts as caught only where deleting `app-uses-ports-not-adapters` removes the
  `&app-deny` anchor.
- So is whether they run (issue #81): `TestGolangciSettingsHideNoIssue` reads `.golangci.yml` with the
  same reader and fails, naming the setting, on every setting that hides issues without touching a
  rule: exclusion paths, a `linters.exclusions.generated` other than `disable` (absent means `lax`),
  `issues` caps and filters, `run.issues-exit-code` outside 1..125 (`os.Exit`
  truncates it to 8 bits, so 256 exits 0), `linters.default` other than `standard` or
  `all`, `run.tests` not true, depguard or forbidigo not enabled or disabled, and an exclusion rule
  naming depguard, or forbidigo without a text or with an empty one. The document's shape is an allow
  list that fails closed: `version` must be `"2"`, and a key or preset it does not know is refused.
  `TestRepositoryReadsOnlyGolangciYML` refuses a `.golangci.json`, `.golangci.toml` or
  `.golangci.yaml` that git could commit, which golangci-lint would read before `.golangci.yml`; the
  lint command lines, in the Makefile and the CI step, are pinned already by `test/architecture`.
- Lint and this check now say the same thing twice: a violation that lint catches also fails here.
