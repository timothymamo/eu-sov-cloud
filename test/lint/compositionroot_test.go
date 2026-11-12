package lint_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// compositionRootTags are the tags of forbidigo's two composition-root groups
// (issue #111). A composition root is the one place that may do what nothing
// else may: cmd/shift2eu and cmd/shift2eu-server build the object graph, and
// only they may hand the process an environment read (ADR 0019). Neither may
// start a program, and neither may dump the environment: the one named variable
// cmd/shift2eu-server/main.go reads is its exception, below.
//
// Group composition-root-no-exec forbids starting a process (os.StartProcess,
// syscall.Exec, syscall.ForkExec, syscall.StartProcess, any use of os/exec) and
// dumping the whole environment (os.Environ, syscall.Environ). It has no
// exception. Group composition-root-env forbids reading a named variable
// (os.Getenv, os.LookupEnv, syscall.Getenv, os.ExpandEnv), with exactly one:
// os.LookupEnv in cmd/shift2eu-server/main.go, which passes it to run for
// SHIFT2EU_SERVER_TOKEN. Both groups hold for files under cmd/shift2eu/ and
// cmd/shift2eu-server/ only, which the groups' linters.exclusions.rules say and
// TestCompositionRootExclusions checks.
var compositionRootTags = []string{"composition-root-no-exec", "composition-root-env"}

// TestCompositionRootForbidigoProbe runs forbidigo, with the repository's
// .golangci.yml, on the two composition roots and a CLI package
// (testdata/compositionroot). Every function the two groups forbid has a line
// in cmd/shift2eu and in cmd/shift2eu-server, over both of that command's
// files, and each must be refused with a msg of the right group. os.LookupEnv
// in cmd/shift2eu-server/main.go must not be refused, while the same reference
// in cmd/shift2eu-server/server.go and in cmd/shift2eu is, so the exception is
// that one file and that one function, and the three other refusal kinds of the
// env group in main.go keep the file from being exempt as a whole. Every call
// the groups refuse, in internal/cli, must pass: a group that lost its
// path-except scope is refused by the probe's strictness, which reports every
// issue the probe does not expect.
//
// The probe module's files call the unix names of syscall, so the probe runs
// where make lint and CI's lint-target matrix run it, on a unix host.
func TestCompositionRootForbidigoProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{dir: "compositionroot", tags: compositionRootTags})
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
}

// noExecAllowed are names a composition root needs, which no
// composition-root-no-exec entry may refuse: naming the process itself, its
// signals and its descriptors, os.Process, and the names that only extend a
// refused one (os.StartProcesses, syscall.Execve, syscall.Environments) catch a
// pattern that lost its $. The three named-variable reads belong to the other
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
	{"os", "os.StartProcesses"},
	{"syscall", "syscall.Execve"},
	{"syscall", "syscall.Environments"},
	{"example.com/otheros", "os.StartProcess"},
	{"example.com/otherexec", "exec.Command"},
}

// envRefused lists every name the composition-root-env group must refuse.
var envRefused = []rootRefused{
	{rootName{"os", "os.Getenv"}, "composition-root-env: "},
	{rootName{"os", "os.LookupEnv"}, "composition-root-env: "},
	{rootName{"os", "os.ExpandEnv"}, "composition-root-env: "},
	{rootName{"syscall", "syscall.Getenv"}, "composition-root-env: "},
}

// envAllowed are names no composition-root-env entry may refuse: the writes, the
// process names, the whole environment (which the other group refuses), the
// names that only extend a refused one (os.Getenvs, os.LookupEnvs,
// os.ExpandEnvs, syscall.Getenvs) catch a pattern that lost its $, and a package
// named os or syscall elsewhere is no composition root's.
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
	{"syscall", "syscall.Getpid"},
	{"os", "os.Getenvs"},
	{"os", "os.LookupEnvs"},
	{"os", "os.ExpandEnvs"},
	{"syscall", "syscall.Getenvs"},
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
	rules := map[string][]exclusionRule{}
	for _, r := range exclusionRules(t, root) {
		if r.linters == nil || !slices.Contains(r.linters, "forbidigo") {
			continue
		}
		tag, ok := groupOfText(r.fields["text"])
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
			var scope []exclusionRule // the rules that drop the whole group
			for _, r := range rules[tag] {
				if len(r.linters) != 1 {
					t.Errorf(".golangci.yml:%d: an exclusion rule of group %s names linters %q; give forbidigo a rule of its own", r.line, tag, r.linters)
				}
				if got, ok := groupOfText(r.fields["text"]); !ok || got != tag {
					t.Errorf(".golangci.yml:%d: exclusion rule text %q is no text of group %s", r.line, r.fields["text"], tag)
				}
				_, byPath := r.fields["path"]
				_, byExcept := r.fields["path-except"]
				if byPath == byExcept {
					t.Errorf(".golangci.yml:%d: an exclusion rule of group %s must have exactly one of path and path-except, and it has %v", r.line, tag, r.fields)
				}
				if r.fields["text"] == groupText(tag) {
					scope = append(scope, r)
					continue
				}
				// A rule that names one entry's msg drops that entry's issues
				// only: an exception, which the env group has exactly one of.
				if tag != "composition-root-env" {
					t.Errorf(".golangci.yml:%d: group %s has an exception for one entry's msg, and it has none: %q", r.line, tag, r.fields["text"])
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
			if _, ok := r.fields["path"]; ok {
				exceptions = append(exceptions, r)
			}
		}
		if len(exceptions) != 1 {
			t.Fatalf("group %s has %d exclusion rules that select a path, want exactly one: the exception for os.LookupEnv in cmd/shift2eu-server/main.go", tag, len(exceptions))
		}
		exception := exceptions[0]
		if text := exception.fields["text"]; text == groupText(tag) {
			t.Errorf(".golangci.yml:%d: the exception's text %q drops every issue of group %s in the files it selects, os.Getenv's and os.ExpandEnv's among them; name the one entry's msg it drops", exception.line, text, tag)
		}

		path := regexp.MustCompile(exception.fields["path"])
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
