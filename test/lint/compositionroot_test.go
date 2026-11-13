package lint_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// compositionRootTags are the tags of forbidigo's two composition-root groups
// (issue #111). A composition root is the one place that may do what nothing
// else may: cmd/shift2eu and cmd/shift2eu-server build the object graph, and
// only they may hand the process an environment read (ADR 0019). Neither may
// start a program, and neither may dump the environment: the one named variable
// cmd/shift2eu-server/main.go reads is its exception, below.
//
// Group composition-root-no-exec forbids starting a process (os.StartProcess,
// os/exec, syscall.Exec, syscall.ForkExec, syscall.StartProcess, the windows
// entry points syscall.CreateProcess and syscall.CreateProcessAsUser, and a DLL
// it loads with syscall.LoadDLL or syscall.LoadLibrary), a raw system call that
// starts one just as well (syscall.Syscall, its 6, 9, 12, 15, 18 and N forms,
// syscall.RawSyscall and syscall.RawSyscall6, and the syscall.SYS_* number one
// takes), a pointer taken apart with package unsafe to pass a system call an
// address, and dumping the whole environment (os.Environ, syscall.Environ,
// syscall.GetEnvironmentStrings). It has no exception. Group composition-root-env
// forbids reading a named variable (os.Getenv, os.LookupEnv, syscall.Getenv,
// syscall.GetEnvironmentVariable, os.ExpandEnv), with exactly one:
// os.LookupEnv in cmd/shift2eu-server/main.go, which passes it to run for
// SHIFT2EU_SERVER_TOKEN. Both groups hold for files under cmd/shift2eu/ and
// cmd/shift2eu-server/ only, which the groups' linters.exclusions.rules say and
// TestCompositionRootExclusions checks.
//
// Every name is a pattern, so a group also holds on the release targets that
// lack a name, such as windows: the probe runs once for the host's platform and
// once for windows, which is where the windows names type-check.
var compositionRootTags = []string{"composition-root-no-exec", "composition-root-env"}

// probeTags are the tags of forbidigo's groups TestCompositionRootForbidigoProbe
// covers: the two composition-root groups, the CLI's own group (issue #152),
// which takes over in internal/cli where the roots' groups stop, and the HTTP
// API's tests group, which holds the one place under internal/httpapi/ no group
// of the two reached before.
var probeTags = slices.Concat(compositionRootTags, []string{cliNoEnvOrProcessTag, httpapiTestsNoEnvOrProcessTag})

// TestCompositionRootForbidigoProbe runs forbidigo, with the repository's
// .golangci.yml, on the two composition roots and a CLI package
// (testdata/compositionroot), twice: once for the host's platform, and once for
// windows, where the windows entry points of syscall type-check. Every function
// the two groups forbid has a line in cmd/shift2eu and in cmd/shift2eu-server,
// over all of that command's files, and each must be refused with a msg of the
// right group. os.LookupEnv in cmd/shift2eu-server/main.go must not be refused,
// while the same reference in cmd/shift2eu-server/server.go and in cmd/shift2eu
// is, so the exception is that one file and that one function, and the three
// other refusal kinds of the env group in main.go keep the file from being
// exempt as a whole. A _test.go file of a composition root must be refused too:
// its tests are compiled into the same package and run with the same
// environment. Every call the CLI's own group refuses, in internal/cli, has a
// line there with a want of that group, the CLI's _test.go file included, and
// the calls internal/cli is for — os opening the inventory a user names, reading
// it, creating the report and taking the FIFO's syscall.Mkfifo — must pass: a
// group that lost its path-except scope, or one of the roots' that lost its own
// scope and reached internal/cli, is refused by the probe's strictness, which
// reports every issue the probe does not expect. The HTTP API's _test.go file is
// held the same way by httpapi-tests-no-env-or-process, which no other group
// reaches: internal/httpapi's production files make no call of the two roots'
// groups, and the calls a test of the handler is for must pass beside them.
//
// A file for one platform builds on that platform only, so each run reads the
// want comments of its own files, and an entry of a group need refuse a line in
// one run only: an entry for the windows names fires in the windows run and one
// for the unix names in the host's, and either counts.
func TestCompositionRootForbidigoProbe(t *testing.T) {
	t.Parallel()

	host := runForbidigoProbe(t, forbidigoProbe{
		dir:             "compositionroot",
		tags:            probeTags,
		files:           probeFilesForHost,
		noEntryCoverage: true,
	})
	windows := runForbidigoProbe(t, forbidigoProbe{
		dir:             "compositionroot",
		tags:            probeTags,
		goos:            "windows",
		goarch:          "amd64",
		packages:        []string{"./cmd/..."},
		files:           probeFilesForWindows,
		noEntryCoverage: true,
	})

	for _, e := range forbidigoEntries(t, moduleRoot(t)) {
		if !slices.ContainsFunc(probeTags, func(tag string) bool { return strings.HasPrefix(e.msg, tag+": ") }) {
			continue
		}
		if !host[e.msg] && !windows[e.msg] {
			t.Errorf(".golangci.yml:%d: no probe line in testdata/compositionroot, for the host's platform or for windows, "+
				"is refused by the entry with msg %q; add a line only it refuses", e.line, e.msg)
		}
	}
}

// probeFilesForHost reports whether the host's run lints the probe file at rel,
// a slash-separated path in the probe module: every file but the ones for
// windows, which name syscall's windows entry points and build there only.
func probeFilesForHost(rel string) bool {
	return !strings.HasSuffix(rel, "_windows.go")
}

// probeFilesForWindows reports whether the windows run lints the probe file at
// rel: the files for windows, and the _test.go files of the commands the run
// lints, which name no platform's function and build for every platform. The
// windows run lints ./cmd/... only, as the files of internal/cli and
// internal/httpapi name unix functions of syscall (the CLI's test takes
// syscall.Mkfifo), and the wants of a file the run does not lint are no wants of
// it.
func probeFilesForWindows(rel string) bool {
	return strings.HasSuffix(rel, "_windows.go") || strings.HasPrefix(rel, "cmd/") && strings.HasSuffix(rel, "_test.go")
}

// rootName is a name as forbidigo renders it, with the import path its pkg is
// matched against: a function as <package>.<name>, and every use of a package
// in os/exec through that package, whichever of its names it uses.
type rootName struct{ pkg, name string }

// rootRefused is a name a group must refuse, with the tag of the group, which
// starts the msg of the entry that must refuse it. The wording of a msg is not
// pinned here: the tag is the contract, and the probe pins that the entry fires.
type rootRefused struct {
	rootName
	tag string
}

// noExecRefused lists every name the composition-root-no-exec group must refuse.
var noExecRefused = []rootRefused{
	{rootName{"os", "os.StartProcess"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.Exec"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.ForkExec"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.StartProcess"}, "composition-root-no-exec: "},
	{rootName{"os/exec", "exec.Command"}, "composition-root-no-exec: "},
	{rootName{"os/exec", "exec.CommandContext"}, "composition-root-no-exec: "},
	{rootName{"os/exec", "exec.LookPath"}, "composition-root-no-exec: "},
	{rootName{"os/exec", "exec.Cmd"}, "composition-root-no-exec: "},
	{rootName{"os", "os.Environ"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.Environ"}, "composition-root-no-exec: "},
	// The windows entry points: a composition root starts no program there
	// either, and loading a DLL runs its code. Every name here exists in
	// syscall on a release target, which is where the probe lints it: a name
	// no target has, such as syscall.LoadLibraryEx, is no pattern's and no
	// probe's to pin.
	{rootName{"syscall", "syscall.CreateProcess"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.CreateProcessAsUser"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.GetEnvironmentStrings"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.LoadDLL"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.MustLoadDLL"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.NewLazyDLL"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.LoadLibrary"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.GetProcAddress"}, "composition-root-no-exec: "},
	// A raw system call starts a process where the kernel call does, and the
	// syscall.SYS_* numbers are what an exec call is made of.
	{rootName{"syscall", "syscall.Syscall"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.Syscall6"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.Syscall9"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.Syscall12"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.Syscall15"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.Syscall18"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.SyscallN"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.RawSyscall"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.RawSyscall6"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.SYS_EXECVE"}, "composition-root-no-exec: "},
	{rootName{"syscall", "syscall.SYS_CLONE"}, "composition-root-no-exec: "},
	// Package unsafe takes a pointer apart, which is how a raw system call is
	// handed an address; a composition root has no pointer it needs it for.
	{rootName{"unsafe", "unsafe.Pointer"}, "composition-root-no-exec: "},
	{rootName{"unsafe", "unsafe.Sizeof"}, "composition-root-no-exec: "},
}

// noExecAllowed are names a composition root needs, which no
// composition-root-no-exec entry may refuse: naming the process itself, its
// signals and its descriptors, os.Process, and the names that only extend a
// refused one (os.StartProcesses, syscall.Execve, syscall.Environments,
// syscall.CreateProcessX, syscall.Syscalls, syscall.RawSyscalls, syscall.SYSCTL)
// catch a pattern that lost its $. The named-variable reads belong to the other
// group, and a package named os or exec elsewhere is no composition root's.
var noExecAllowed = []rootName{
	{"os", "os.Exit"},
	{"os", "os.Args"},
	{"os", "os.Stdout"},
	{"os", "os.Stderr"},
	{"os", "os.Interrupt"},
	{"os", "os.Getpid"},
	{"os", "os.Getpagesize"},
	{"os", "os.Process"},
	{"os", "os.FindProcess"},
	{"os", "os.Getenv"},
	{"os", "os.LookupEnv"},
	{"os", "os.ExpandEnv"},
	{"syscall", "syscall.SIGTERM"},
	{"syscall", "syscall.SIGHUP"},
	{"syscall", "syscall.Kill"},
	{"syscall", "syscall.Wait4"},
	{"syscall", "syscall.Getpid"},
	{"syscall", "syscall.GetEnvironmentVariable"},
	{"os", "os.StartProcesses"},
	{"syscall", "syscall.Execve"},
	{"syscall", "syscall.Environments"},
	{"syscall", "syscall.CreateProcessX"},
	{"syscall", "syscall.Syscalls"},
	{"syscall", "syscall.RawSyscalls"},
	{"syscall", "syscall.SYSCTL"},
	{"example.com/otheros", "os.StartProcess"},
	{"example.com/otherexec", "exec.Command"},
}

// envRefused lists every name the composition-root-env group must refuse: every
// read of a named variable, and the four lookups that locate a home, config, cache
// or temp directory from an unnamed one (issue #152, the auditor's finding M-B:
// without them a root could read ~/.aws/credentials with lint green).
var envRefused = []rootRefused{
	{rootName{"os", "os.Getenv"}, "composition-root-env: "},
	{rootName{"os", "os.LookupEnv"}, "composition-root-env: "},
	{rootName{"os", "os.ExpandEnv"}, "composition-root-env: "},
	{rootName{"os", "os.UserHomeDir"}, "composition-root-env: "},
	{rootName{"os", "os.UserConfigDir"}, "composition-root-env: "},
	{rootName{"os", "os.UserCacheDir"}, "composition-root-env: "},
	{rootName{"os", "os.TempDir"}, "composition-root-env: "},
	{rootName{"syscall", "syscall.Getenv"}, "composition-root-env: "},
	{rootName{"syscall", "syscall.GetEnvironmentVariable"}, "composition-root-env: "},
}

// envAllowed are names no composition-root-env entry may refuse: the writes, the
// process names, the whole environment (which the other group refuses, on unix
// as syscall.Environ and on windows as syscall.GetEnvironmentStrings), the names
// that only extend a refused one (os.Getenvs, os.LookupEnvs, os.ExpandEnvs,
// os.UserHomeDirs, os.UserConfigDirs, os.UserCacheDirs, os.TempDirs,
// syscall.Getenvs, syscall.GetEnvironmentVariables) catch a pattern that lost
// its $, and a package named os or syscall elsewhere is no composition root's.
var envAllowed = []rootName{
	{"os", "os.Setenv"},
	{"os", "os.Unsetenv"},
	{"os", "os.Clearenv"},
	{"os", "os.Environ"},
	{"os", "os.Exit"},
	{"os", "os.Getpid"},
	{"os", "os.Getpagesize"},
	{"syscall", "syscall.Setenv"},
	{"syscall", "syscall.Unsetenv"},
	{"syscall", "syscall.Environ"},
	{"syscall", "syscall.GetEnvironmentStrings"},
	{"syscall", "syscall.Getpid"},
	{"os", "os.Getenvs"},
	{"os", "os.LookupEnvs"},
	{"os", "os.ExpandEnvs"},
	{"os", "os.UserHomeDirs"},
	{"os", "os.UserConfigDirs"},
	{"os", "os.UserCacheDirs"},
	{"os", "os.TempDirs"},
	{"syscall", "syscall.Getenvs"},
	{"syscall", "syscall.GetEnvironmentVariables"},
	{"example.com/otheros", "os.Getenv"},
	{"example.com/othersyscall", "syscall.Getenv"},
}

// TestCompositionRootForbidigoPatterns checks the entries of the two
// composition-root groups, read from .golangci.yml, offline and without
// golangci-lint: every name a group must refuse is refused by an entry of it, no
// name it must leave alone is refused by one, and every entry refuses a listed
// name. The probe proves how forbidigo renders a name; this pins the patterns
// and their packages over more names than a probe can hold.
func TestCompositionRootForbidigoPatterns(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)

	tests := []struct {
		name    string
		tag     string
		refused []rootRefused
		allowed []rootName
	}{
		{
			name:    "the no-exec group refuses every way to start a program and to dump the environment",
			tag:     "composition-root-no-exec",
			refused: noExecRefused,
			allowed: noExecAllowed,
		},
		{
			name:    "the env group refuses every way to read one named variable",
			tag:     "composition-root-env",
			refused: envRefused,
			allowed: envAllowed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			entries := groupEntries(t, root, tc.tag)
			refuses := func(e forbidEntry, n rootName) bool {
				return e.pkg.MatchString(n.pkg) && e.pattern.MatchString(n.name)
			}

			for _, r := range tc.refused {
				var msgs []string
				for _, e := range entries {
					if refuses(e, r.rootName) {
						msgs = append(msgs, e.msg)
					}
				}
				if !slices.ContainsFunc(msgs, func(m string) bool { return strings.HasPrefix(m, r.tag) }) {
					t.Errorf("%s (pkg %q) must be refused by an entry of group %s; refused by %q", r.name, r.pkg, r.tag, msgs)
				}
			}
			for _, n := range tc.allowed {
				for _, e := range entries {
					if refuses(e, n) {
						t.Errorf("%s (pkg %q) must pass a composition root, but the entry with msg %q refuses it", n.name, n.pkg, e.msg)
					}
				}
			}
			for _, e := range entries {
				if !slices.ContainsFunc(tc.refused, func(r rootRefused) bool { return refuses(e, r.rootName) }) {
					t.Errorf("the entry with msg %q refuses no name of the group's list; add the names it refuses", e.msg)
				}
			}
		})
	}
}

// TestCompositionRootExclusions checks the linters.exclusions.rules of the two
// composition-root groups, read from .golangci.yml, offline and without
// golangci-lint. Each rule names forbidigo alone and carries a text in the
// documented form of its group, so it drops one group's issues and no other's.
// Both groups hold for files under cmd/shift2eu/ and cmd/shift2eu-server/ only,
// so every file elsewhere passes them. The composition-root-env group has
// exactly one exception, on one path: it selects cmd/shift2eu-server/main.go and
// no other file, its text names the msg of the one entry that refuses
// os.LookupEnv, and it drops no other entry's issue there, so the exception is
// os.LookupEnv in that file alone and not the group in it. The no-exec group has
// no exception, and neither group names a path a single file is the whole of,
// which is what a blanket allow would look like.
func TestCompositionRootExclusions(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	all := exclusionRules(t, root)
	rules := map[string][]exclusionRule{}
	for _, r := range all {
		if r.linters == nil || !slices.Contains(r.linters, "forbidigo") {
			continue
		}
		tag, ok := groupOfText(t, root, r.fields["text"])
		if !ok || !slices.Contains(compositionRootTags, tag) {
			continue // a rule of another group, or one TestForbidigoExclusionsDropOneGroup reports
		}
		rules[tag] = append(rules[tag], r)
	}

	paths := []struct {
		path string
		in   bool // whether both groups hold for it
	}{
		{"cmd/shift2eu/main.go", true},
		{"cmd/shift2eu/main_test.go", true},
		{"cmd/shift2eu-server/main.go", true},
		{"cmd/shift2eu-server/run.go", true},
		{"internal/cli/cli.go", false},
		{"internal/httpapi/handler.go", false},
		{"internal/adapter/source/aws/aws.go", false},
		{"cmd/shift2eux/main.go", false},
		{"cmd/shift2eu.go", false},
		{"main.go", false},
	}

	for _, tag := range compositionRootTags {
		t.Run(tag, func(t *testing.T) {
			t.Parallel()

			if len(rules[tag]) == 0 {
				t.Fatalf(".golangci.yml holds no linters.exclusions.rules entry of group %s, so the group would hold everywhere", tag)
			}
			for _, problem := range compositionRootRuleProblems(t, root, tag, all) {
				t.Error(problem)
			}
			var scope []exclusionRule // the rules that drop the whole group
			for _, r := range rules[tag] {
				if r.fields["text"] == groupText(tag) {
					scope = append(scope, r)
				}
			}
			if len(scope) == 0 {
				t.Errorf("group %s has no exclusion rule that drops the whole group, so nothing scopes it to the composition roots", tag)
			}

			for _, p := range paths {
				if got := inScope(t, scope, p.path); got != p.in {
					t.Errorf("group %s must hold for %s: %t, and the group's exclusion rules make it %t", tag, p.path, p.in, got)
				}
			}
		})
	}

	t.Run("the one exception", func(t *testing.T) {
		t.Parallel()

		const tag = "composition-root-env"

		var exceptions []exclusionRule
		for _, r := range rules[tag] {
			if r.fields["text"] != groupText(tag) {
				exceptions = append(exceptions, r)
			}
		}
		if len(exceptions) != 1 {
			t.Fatalf("group %s has %d exclusion rules that drop one entry's msg rather than the whole group, want exactly one: the exception for os.LookupEnv in cmd/shift2eu-server/main.go", tag, len(exceptions))
		}
		exception := exceptions[0]
		if text := exception.fields["text"]; text == groupText(tag) {
			t.Errorf(".golangci.yml:%d: the exception's text %q drops every issue of group %s in the files it selects, os.Getenv's and os.ExpandEnv's among them; name the one entry's msg it drops", exception.line, text, tag)
		}

		selected, ok := exception.fields["path"]
		if !ok {
			t.Fatalf(".golangci.yml:%d: the exception selects no path: %q", exception.line, exception.fields)
		}
		path := regexp.MustCompile(selected)
		if !path.MatchString("cmd/shift2eu-server/main.go") {
			t.Errorf(".golangci.yml:%d: the exception's path %q does not select cmd/shift2eu-server/main.go", exception.line, exception.fields["path"])
		}
		for _, other := range []string{
			"cmd/shift2eu-server/main_test.go",
			"cmd/shift2eu-server/run.go",
			"cmd/shift2eu-server/token.go",
			"cmd/shift2eu/main.go",
			"internal/cli/main.go",
		} {
			if path.MatchString(other) {
				t.Errorf(".golangci.yml:%d: the exception's path %q also selects %s, so it is no exception for one file", exception.line, exception.fields["path"], other)
			}
		}

		// The exception drops the issue of the entry that refuses
		// os.LookupEnv, and no other entry's issue of the group.
		entries := groupEntries(t, root, tag)
		text := regexp.MustCompile(exception.fields["text"])
		var lookup, others []string
		for _, e := range entries {
			if e.pkg.MatchString("os") && e.pattern.MatchString("os.LookupEnv") {
				lookup = append(lookup, e.msg)
			} else {
				others = append(others, e.msg)
			}
		}
		if len(lookup) == 0 {
			t.Fatalf(".golangci.yml holds no entry of group %s that refuses os.LookupEnv, so the exception drops nothing", tag)
		}
		if len(others) == 0 {
			t.Fatalf(".golangci.yml holds no other entry of group %s, so a blanket allow would pass this check", tag)
		}
		for _, msg := range lookup {
			if !text.MatchString(issueText("os.LookupEnv", msg)) {
				t.Errorf(".golangci.yml:%d: the exception's text %q does not drop the issue of the entry with msg %q, so it drops nothing of the one call it is for", exception.line, exception.fields["text"], msg)
			}
		}
		for _, msg := range others {
			if text.MatchString(issueText("os.LookupEnv", msg)) || text.MatchString(issueText("os.Getenv", msg)) {
				t.Errorf(".golangci.yml:%d: the exception's text %q drops the issue of the entry with msg %q too, so the exception is a blanket allow of the group in that file, not one function", exception.line, exception.fields["text"], msg)
			}
		}
		for _, e := range groupEntries(t, root, "composition-root-no-exec") {
			if text.MatchString(issueText("os.Environ", e.msg)) {
				t.Errorf(".golangci.yml:%d: the exception's text %q drops the issue of the entry with msg %q, which is no text of group %s", exception.line, exception.fields["text"], e.msg, tag)
			}
		}
	})
}

// compositionRootRuleProblems returns what is wrong with rules, the
// linters.exclusions.rules entries of .golangci.yml whose text is a text of the
// composition-root group with tag, one line each:
//   - a rule names forbidigo alone, so it drops no other linter's issues;
//   - it has exactly one of path and path-except;
//   - a rule whose text is not the group's whole form is an exception: it drops
//     one entry's issues, so it selects a path, never a path-except. A
//     path-except would drop that entry's issues in every file but the
//     directories it names, which is the opposite of an exception, and no other
//     check reads it: only a whole-group rule scopes its group, so a narrowed
//     path-except rule would widen what the group allows and pass unseen;
//   - the composition-root-env group, the one group with an exception, has
//     exactly one, and no other group of the two has any.
func compositionRootRuleProblems(t *testing.T, root, tag string, rules []exclusionRule) []string {
	t.Helper()

	var problems []string
	exceptions := 0
	for _, r := range rules {
		if got, ok := groupOfText(t, root, r.fields["text"]); !ok || got != tag {
			continue // a rule of another group, or of no group
		}
		if len(r.linters) != 1 || r.linters[0] != "forbidigo" {
			problems = append(problems, fmt.Sprintf(".golangci.yml:%d: an exclusion rule of group %s names linters %q; give forbidigo a rule of its own", r.line, tag, r.linters))
		}
		_, byPath := r.fields["path"]
		_, byExcept := r.fields["path-except"]
		if byPath == byExcept {
			problems = append(problems, fmt.Sprintf(".golangci.yml:%d: an exclusion rule of group %s must have exactly one of path and path-except, and it has %v", r.line, tag, r.fields))
		}
		if r.fields["text"] == groupText(tag) {
			continue
		}
		exceptions++
		if !byPath {
			problems = append(problems, fmt.Sprintf(".golangci.yml:%d: group %s has one exception, for one entry of the group in one file, and this rule drops %s everywhere but the files its path-except names: an exception selects its file with path", r.line, tag, r.fields["text"]))
		}
	}
	switch {
	case tag == "composition-root-env" && exceptions != 1:
		problems = append(problems, fmt.Sprintf("group %s has %d exclusion rules that drop one entry's msg and not the whole group, want exactly one: the exception for os.LookupEnv in cmd/shift2eu-server/main.go", tag, exceptions))
	case tag != "composition-root-env" && exceptions > 0:
		problems = append(problems, fmt.Sprintf("group %s has no exception, and %d of its exclusion rules drop one entry's msg instead of the whole group", tag, exceptions))
	}
	return problems
}

// TestCompositionRootRuleProblems pins compositionRootRuleProblems, the seam
// that reads the exclusion rules of the two composition-root groups
// (TestCompositionRootExclusions): the rules of a document no test reads from
// disk fail here, and the failure is the pin. A narrowed rule with a
// path-except in particular passes every check of the group's scope, as only a
// whole-group rule is read for it, so it is pinned here.
func TestCompositionRootRuleProblems(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	const tag = "composition-root-env"
	scope := func(tag string) exclusionRule {
		return exclusionRule{
			line:    4,
			fields:  map[string]string{"path-except": `^cmd/shift2eu(?:-server)?/`, "text": groupText(tag)},
			linters: []string{"forbidigo"},
		}
	}
	// The msg of the one entry of the env group that refuses os.LookupEnv, and
	// of the first entry of the no-exec group: the exceptions below name a real
	// entry's msg, as a narrowed rule does.
	var lookupEnvMsg, noExecMsg string
	for _, e := range groupEntries(t, root, tag) {
		if e.pkg.MatchString("os") && e.pattern.MatchString("os.LookupEnv") {
			lookupEnvMsg = e.msg
		}
	}
	if lookupEnvMsg == "" {
		t.Fatalf(".golangci.yml holds no entry of group %s that refuses os.LookupEnv", tag)
	}
	noExecMsg = groupEntries(t, root, "composition-root-no-exec")[0].msg
	exception := exclusionRule{
		line:    9,
		fields:  map[string]string{"path": `^cmd/shift2eu-server/main\.go$`, "text": narrowedTextOf(lookupEnvMsg)},
		linters: []string{"forbidigo"},
	}

	tests := []struct {
		name  string
		tag   string
		rules []exclusionRule
		want  []string
	}{
		{
			name:  "the whole group on a path-except scopes it to the composition roots",
			tag:   tag,
			rules: []exclusionRule{scope(tag), exception},
		},
		{
			// The hole this pins: the rule drops the os.LookupEnv entry in
			// every file but the two it names as path-except, so it widens what
			// the env group allows there and no scope check reads it.
			name: "a narrowed rule with a path-except is refused",
			tag:  tag,
			rules: []exclusionRule{scope(tag), {
				line:    9,
				fields:  map[string]string{"path-except": `^cmd/shift2eu(?:-server)?/(?:main|server)\.go$`, "text": narrowedTextOf(lookupEnvMsg)},
				linters: []string{"forbidigo"},
			}},
			want: []string{
				`.golangci.yml:9: group composition-root-env has one exception, for one entry of the group in one file, and this rule drops because "composition-root-env: os\.LookupEnv is called in cmd/shift2eu-server/main\.go only, for SHIFT2EU_SERVER_TOKEN, in the token source it hands to run \(ADR 0019\); take any other value as a flag"$ everywhere but the files its path-except names: an exception selects its file with path`,
			},
		},
		{
			name: "two exceptions are refused",
			tag:  tag,
			rules: []exclusionRule{scope(tag), exception, {
				line:    14,
				fields:  map[string]string{"path": `^cmd/shift2eu/main\.go$`, "text": narrowedTextOf(lookupEnvMsg)},
				linters: []string{"forbidigo"},
			}},
			want: []string{
				"group composition-root-env has 2 exclusion rules that drop one entry's msg and not the whole group, want exactly one: the exception for os.LookupEnv in cmd/shift2eu-server/main.go",
			},
		},
		{
			name: "an exception of the no-exec group is refused",
			tag:  "composition-root-no-exec",
			rules: []exclusionRule{scope("composition-root-no-exec"), {
				line:    9,
				fields:  map[string]string{"path": `^cmd/shift2eu/main\.go$`, "text": narrowedTextOf(noExecMsg)},
				linters: []string{"forbidigo"},
			}},
			want: []string{
				"group composition-root-no-exec has no exception, and 1 of its exclusion rules drop one entry's msg instead of the whole group",
			},
		},
		{
			name: "a scope rule that names a second linter is refused",
			tag:  tag,
			rules: []exclusionRule{{
				line:    4,
				fields:  map[string]string{"path-except": `^cmd/shift2eu(?:-server)?/`, "text": groupText(tag)},
				linters: []string{"forbidigo", "gosec"},
			}, exception},
			want: []string{
				`.golangci.yml:4: an exclusion rule of group composition-root-env names linters ["forbidigo" "gosec"]; give forbidigo a rule of its own`,
			},
		},
		{
			name: "a rule that selects nothing is refused",
			tag:  tag,
			rules: []exclusionRule{{
				line:    4,
				fields:  map[string]string{"text": groupText(tag)},
				linters: []string{"forbidigo"},
			}, exception},
			want: []string{
				`.golangci.yml:4: an exclusion rule of group composition-root-env must have exactly one of path and path-except, and it has map[text:because "composition-root-env: [^"]*"$]`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := compositionRootRuleProblems(t, root, tt.tag, tt.rules)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("compositionRootRuleProblems() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// inScope reports whether the group whose exclusion rules are rules holds for
// path: golangci-lint drops an issue when a rule's path matches it, or when its
// path-except does not, so the group holds where no rule drops it.
func inScope(t *testing.T, rules []exclusionRule, path string) bool {
	t.Helper()

	for _, r := range rules {
		if p, ok := r.fields["path"]; ok && regexp.MustCompile(p).MatchString(path) {
			return false
		}
		if p, ok := r.fields["path-except"]; ok && !regexp.MustCompile(p).MatchString(path) {
			return false
		}
	}
	return true
}

// issueText is the text golangci-lint reports for a use of name, which the entry
// with msg refuses: what an exclusion rule's text is matched against.
func issueText(name, msg string) string {
	return "use of `" + name + "` forbidden because \"" + msg + "\""
}

// forbidEntry is one forbidigo entry of .golangci.yml, its pattern and pkg
// compiled for matching a rendered name, as TestCloudReadOnlyPatterns and
// TestDomainIsPurePatterns match them.
type forbidEntry struct {
	pattern, pkg *regexp.Regexp
	msg          string
}

// groupEntries returns the entries of the forbidigo group with tag, compiled,
// and fails when the group holds none.
func groupEntries(t *testing.T, root, tag string) []forbidEntry {
	t.Helper()

	var entries []forbidEntry
	for _, e := range forbidigoEntries(t, root) {
		if !strings.HasPrefix(e.msg, tag+": ") {
			continue
		}
		pattern, err := regexp.Compile(e.pattern)
		if err != nil {
			t.Fatalf(".golangci.yml:%d: pattern: %v", e.line, err)
		}
		pkg, err := regexp.Compile(e.pkg)
		if err != nil {
			t.Fatalf(".golangci.yml:%d: pkg: %v", e.line, err)
		}
		entries = append(entries, forbidEntry{pattern: pattern, pkg: pkg, msg: e.msg})
	}
	if len(entries) == 0 {
		t.Fatalf(".golangci.yml holds no %s entry", tag)
	}
	return entries
}
