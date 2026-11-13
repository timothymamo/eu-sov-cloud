// Package lint holds rules on the repository's Go source that golangci-lint
// cannot express, written as plain tests, and the probe that proves a
// golangci-lint rule still fires. make lint runs them after golangci-lint, and
// as they carry no build tag, make test, make check and make cover (CI's Test
// job) run them too. It holds doc.go and _test.go files only, which the strict
// depguard rule test-docs-are-pure keeps so: the checker the shellcheck
// directive rule below runs lives in test/internal/shellcheck (issue #177).
//
// TestTrackedShellScriptsNameTheirShellcheckCodes holds every shell script git
// tracks (shellcheck.IsShellScript: a .sh file, or a sh or bash shebang, the
// hooks under .githooks among them) to the rule that a shellcheck disable
// directive names the codes it silences, each SC and four digits, never all,
// and gives its reason after a second "#" on the same line (issue #164), as
// nolintlint does for a Go nolint directive. shellcheck.CheckDirectives is the
// checker; make lint runs shellcheck and actionlint themselves.
//
// TestSourcesHideNothingFromDepguard reads every Go file the go command's
// ./... pattern reaches, whatever its build constraints, so every file make
// lint lints in any of its golangci-lint runs (native and one per release
// target), and fails on each way a file could hide code from depguard, which
// enforces the dependency rule (docs/architecture.md), or from forbidigo,
// which enforces the cloud-read-only rule (CLAUDE.md rule 1):
//
//   - A nolint directive that would hide a depguard or a forbidigo issue: one
//     that names depguard or forbidigo, one that names all, and one that names
//     no linter. nolintlint, configured in .golangci.yml, requires a directive
//     to name its linters and give a reason, but it cannot refuse one named
//     linter, and it lets some directives that silence every linter through,
//     such as //nolint: all with a space after the colon.
//   - A //go:build or // +build line that negates the e2e or integration tag.
//     Every lint run sets both, so lint would never read a file behind !e2e
//     that a build without the tag compiles.
//   - A Go file that is a symbolic link: golangci-lint lints it with the
//     directives of its target, wherever that lies.
//
// The negated tags are those of run.build-tags in .golangci.yml, which
// TestNegatableTagsMatchGolangci keeps in step; the go command runs below take
// their -tags from the same list. test/architecture refuses the other ways:
// TestRepositoryHasNoLineDirectives fails on a //line directive, whose
// position depguard's file globs would match, and on any symlink git tracks or
// would add. Refusing symlinked Go files here too is deliberate defence in
// depth: this walk does not depend on git and also reaches files git ignores.
// Every test here that reads Go files, of the module or of a probe, reads them
// through this one walk, walkGoFiles, so each refuses a symlinked Go file.
// docs/architecture.md lists every way and its check in one place.
//
// TestCloudReadOnlyProbe runs the golangci-lint version CI pins, forbidigo and
// depguard, on probe source adapters over stub cloud SDKs
// (testdata/cloudreadonly), offline and with no credentials. Each probe line
// that ends in a want comment must be refused with the messages it names, no
// other line may be, and every entry of forbidigo's cloud-read-only group must
// refuse a line of its own, so a pattern that silently stops matching fails.
// The harness, runForbidigoProbe, serves any forbidigo group. Where
// golangci-lint is not on PATH (CI's Test job) it skips, unless
// SHIFT2EU_LINT_PROBES=require, which make lint and every entry of CI's
// lint-target matrix (the Lint check) set. Only CI fails on a golangci-lint
// version other than the pinned one; elsewhere the probe runs and logs a
// warning. A probe with no forbidigo tag runs depguard alone:
// TestAdaptersBuiltInCmdProbe (testdata/adaptersbuiltincmd) imports an adapter
// from each subtree of internal/adapter, and each subtree's root package, in
// packages outside the composition roots, and must be refused by
// adapters-are-built-in-cmd, so a deny entry shadowed by another, as in issue
// #119, fails. TestAppTestsUseFakesNotAdaptersProbe (testdata/apptestimports)
// imports, in an application-layer _test.go file, the standard packages that
// do file, network or process I/O, os and net among them and those that need
// neither (io/ioutil, text/template, debug/elf, runtime/debug, go/parser,
// mime/multipart, runtime/coverage, unsafe, ...), and packages on no list
// (encoding/xml, expvar) or pure but unneeded (runtime, go/build/constraint),
// and must be refused by the strict app-tests-use-fakes-not-adapters rule
// (issue #144), while every package its allow list names (bytes, errors,
// strings, context, testing, go-cmp, internal/testutil, ...) must not: the
// rule refuses everything its allow list leaves out, so the probe pins both
// the allow list and that no deny entry shadows an allowed package. fmt and
// time, which the allow list no longer names, must be refused as well. The
// probe runs twice: for the host with cgo on, and for darwin/amd64 with cgo
// off, a release target that is not the host's. TestTestutilImportsProbe
// (testdata/testutilimports) holds internal/testutil, production and _test.go
// files alike, to the strict testutil-imports rule (net/http and os/exec are
// refused, os and path/filepath for the golden files pass) and runs forbidigo's
// testutil-no-env group on env.go, whose environment reads, home, config,
// cache and temp lookups and os.StartProcess must each be refused (the writes
// are tests-no-process-state's since issue #153); TestTestutilNoEnvForbidigoPatterns
// pins that group's pattern against lists of refused and allowed names.
// TestCLIImportsProbe (testdata/cliimports) holds internal/cli to the strict
// cli-gets-adapters-from-cmd rule (issue #152): os/exec, syscall and unsafe are
// refused in its production files, the packages the CLI imports today pass; the
// forbidigo group cli-no-env-or-process refuses the CLI's environment reads,
// home, config, cache and temp lookups, os.StartProcess and, for its tests,
// which import syscall for syscall.Mkfifo, the roots' syscall names, on the
// internal/cli of TestCompositionRootForbidigoProbe's module (an aliased os
// import included), and TestCLINoEnvOrProcessForbidigoPatterns pins its
// patterns, the windows names offline. The group
// httpapi-tests-no-env-or-process does the same for internal/httpapi's
// _test.go files, on the same module's internal/httpapi, and
// TestHTTPAPITestsNoEnvOrProcessForbidigoPatterns pins it.
// TestVersionImportsProbe (testdata/versionimports) holds internal/version to
// the strict version-is-build-metadata rule: fmt passes, os and os/exec are
// refused. TestContractSuitesArePureProbe (testdata/contractsuites) holds the
// contract suites sourcetest and targettest to the strict
// contract-suites-are-pure rule (issue #152): net, os, os/exec, syscall and
// unsafe are refused, the packages they import today pass.
// TestInventoryFileNoEnvProbe (testdata/inventoryfile) runs the forbidigo group
// inventoryfile-no-env (issue #152) on the inventory-file reader, whose
// environment reads, home, config, cache and temp lookups and os.StartProcess
// must each be refused, and TestInventoryFileNoEnvForbidigoPatterns pins its
// pattern.
// TestE2ENoEnvProbe (testdata/e2enoenv) runs the forbidigo group e2e-no-env
// (issue #167) on the e2e tier, whose os and syscall environment reads, process
// starts and raw system calls, and (*exec.Cmd).Environ (issue #185), must each
// be refused while os/exec stays allowed, and TestE2ENoEnvForbidigoPatterns pins
// its patterns. TestE2EImportsProbe (testdata/e2eimports) runs depguard on the
// e2e tier (issue #185): a non-test file importing os/exec, unsafe or os/user is
// refused by test-docs-are-pure, and a _test.go file importing os/exec is not.
//
// TestE2ETestsSetExecEnv and TestTestTreeNamesNoProcEnviron close what the
// e2e tier's two rules above cannot see (issue #195). The e2e _test.go files
// start children, so depguard cannot keep os/exec from them, and e2e-no-env
// refuses the one call that dumps the environment but no command built with a
// nil Env, which inherits all of it: the agents' keys and tokens live there,
// and the child's output can reach a t.Log call or a golden file.
// TestE2ETestsSetExecEnv runs execenv.CheckExecEnv over every
// test/e2e/*_test.go and fails on each command built by exec.Command or
// exec.CommandContext whose Run, Start, Output or CombinedOutput call comes
// before the command's Env was assigned in the same function, unless the
// command's name is on the exemption list the test declares: the go
// toolchain's `go build` and `go list` calls, which need that environment and
// print none of it. TestTestTreeNamesNoProcEnviron runs
// execenv.CheckEnvironLiterals over every Go file under test/, testdata
// directories included, and fails on a string literal naming a
// /proc/<something>/environ path, which reads the same environment without a
// child at all. Both checkers and their probes are in test/internal/execenv,
// whose probes are .go.txt files, so this scan never meets a refused literal
// it planted itself.
// TestTestsNoProcessStateProbe (testdata/testsprocessstate) runs the forbidigo
// group tests-no-process-state (issue #153) on t.Setenv, t.Chdir, os.Setenv,
// os.Unsetenv, os.Clearenv, os.Chdir, syscall.Unsetenv, syscall.Clearenv and
// syscall.Fchdir, and on a struct that embeds
// *testing.T, in the application layer, the test helpers, the driving
// adapters, the roots, a source adapter, the domain and the e2e tier, where
// each must be refused, while t.TempDir, os.MkdirTemp and os.CreateTemp pass;
// its first entry matches any Setenv or Chdir whatever the receiver.
// TestTestsNoProcessStateForbidigoPatterns pins its patterns.
// forbidigo renders no call through an interface literal or a type
// parameter, so TestTestsChangeNoProcessStateByNoSelector refuses every
// selector named Setenv, Chdir, Unsetenv, Clearenv or Fchdir in the group's
// scope, every file but those of test/lint and test/architecture, on the AST,
// and
// TestProcessStateSelectorFindings pins that check on fixtures.
// TestCloudReadOnlyPatterns and
// TestCloudReadOnlyPackages check the group's allowlist patterns, read from
// .golangci.yml, against lists of write and read names and of import paths,
// without golangci-lint. TestForbidigoExclusionsDropOneGroup checks every
// exclusion rule that drops forbidigo issues: it names forbidigo alone, its
// text matches only one group's quoted msg, and the rules of each group are
// exactly those it states: none for cloud-read-only, and for domain-is-pure
// one path and one path-except. Two groups hold the composition roots
// cmd/shift2eu and cmd/shift2eu-server (issue #111): composition-root-no-exec
// refuses starting a process and dumping the environment (os.StartProcess,
// os/exec, syscall.Exec, ForkExec, StartProcess, os.Environ, syscall.Environ),
// the windows entry points (syscall.CreateProcess, CreateProcessAsUser,
// GetEnvironmentStrings, LoadDLL, MustLoadDLL, NewLazyDLL, LoadLibrary,
// GetProcAddress), the raw system calls (syscall.Syscall,
// Syscall6, 9, 12, 15, 18 and N, RawSyscall, RawSyscall6) and syscall.SYS_*,
// and any use of unsafe; composition-root-env refuses reading a named variable
// (os.Getenv, os.LookupEnv, os.ExpandEnv, syscall.Getenv,
// syscall.GetEnvironmentVariable) and, since issue #152, locating a home,
// config, cache or temp directory (os.UserHomeDir, os.UserConfigDir,
// os.UserCacheDir, os.TempDir). The probe lints the roots natively and for
// windows/amd64. Those names are defence in depth: the comprehensive guard is
// TestCompositionRootsUseSyscallOnlyForSignals, an AST check under which the
// roots, tests included, use package syscall for a ^SIG[A-Z0-9]+$ constant and
// the type syscall.Signal only, and TestRootsAndCLILinkNoForeignSymbol
// refuses //go:linkname in them and in both driving adapters, internal/cli
// and internal/httpapi. TestCompositionRootPassesLookupEnvToRun holds
// cmd/shift2eu-server/main.go to one os.LookupEnv call, with the identifier
// tokenEnvVar, as the sole return of the func() (string, bool) token source it
// passes to run, so run reads no other variable, and
// TestRootsAndCLINameNoEnvironmentPath refuses a literal holding the path
// segment proc or dev/fd, at its start or after a /, with or without a
// trailing /, but not myproc, in the roots, both driving adapters and the
// inventory-file reader (proc/self/environ read through os.DirFS("/") has no
// leading slash, and os.DirFS("/proc") no trailing one); no group can state
// either.
// The depguard rule composition-root-tests refuses go/build, net/http/cgi,
// os/exec, os/user, plugin, runtime/cgo, syscall/js and unsafe in the roots'
// _test.go files, which the strict composition-root rules skip, and in the
// driving adapters' _test.go files; TestCompositionRootTestsReachNoForeignCodeProbe
// runs it on a probe module (testdata/compositionroottests). TestCompositionRootExclusions
// pins their one path-except each and the env group's one exception, whose text
// is the group's form with the os.LookupEnv entry's exact msg in place of
// [^"]*, so it drops that entry's issue in cmd/shift2eu-server/main.go alone;
// TestCompositionRootForbidigoPatterns and TestCompositionRootForbidigoProbe
// pin their entries. It reads linters.exclusions.rules with
// test/internal/yamlread, the strict line reader test/architecture reads
// .golangci.yml with, so a rule written in a way it does not follow fails.
//
// TestLintLoadsEveryGoFile and TestLintVisitsEveryImportedPackage keep every
// Go file within lint's reach. Lint loads, for the host and every release
// target of .goreleaser.yaml, what builds with the tags e2e and integration,
// and ./... skips testdata, _ and . directories, so a file behind another build
// tag, or an imported package in a skipped directory, would run unchecked, with
// real credentials in an integration test. The tests run the go command
// offline. TestLintLoadsEveryGoFile reads the release targets from
// .goreleaser.yaml with test/internal/releasetarget, the reader
// test/architecture holds the release to, and counts a file as loaded when the
// go command loads it for any of them with cgo off and the baseline GOAMD64
// and GOARM64 levels, as the release builds them; a file only for one release
// platform passes, and a platform the release gains is one the test holds
// files to. A file only a cgo build compiles is refused on purpose: the
// release builds with CGO_ENABLED=0, and only the release targets count
// toward loaded, not make lint's native run with cgo on, which the release
// never ships. TestUnloadedGoFilesProbe pins
// its helper on a probe module (testdata/platformprobe).
//
// TestNoInterfaceDeclaresACloudWrite and TestNoTypeHidesACloudSDKType close
// what forbidigo's cloud-read-only group cannot see. An interface a cloud
// source adapter declares, generic constraints and interface literals
// included, may declare only read verbs or unexported methods, and an interface
// elsewhere no method named for a write; no struct embeds an SDK type.
// TestInterfaceFindingsProbe runs the interface rule on the probe, and
// TestWriteVerbsMatchForbidigo keeps the write verbs the same here and in
// .golangci.yml.
package lint
