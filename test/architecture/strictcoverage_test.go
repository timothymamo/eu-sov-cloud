package architecture_test

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
	"github.com/timothymamo/shift2eu/test/internal/releasetarget"
	"github.com/timothymamo/shift2eu/test/internal/yamlread"
)

// modulePackageDirs returns the directory, relative to the module root, of every
// non-test module package a composition root could hand the work to. It lists
// them with go list for every release target .goreleaser.yaml builds, with the
// e2e and integration files on as make lint loads them, so a package that only
// builds on one platform or one build tag is listed too.
//
// The listing takes the whole module and not internal/ and cmd/ alone (issue
// #152, the auditor's finding H-1): the roots' allow lists name the module path
// as a prefix entry, so cmd/shift2eu or a test of internal/cli may import
// test/lint, test/devenv or a new top-level pkg/ as well, and the earlier
// listing of internal/ and cmd/ alone left those packages to no rule at all.
// Only Go's own internal rule keeps a package out of reach of both roots, and
// only that rule is exempted here: a package whose path holds an internal
// element whose parent is not the module root (test/internal/...,
// test/architecture/internal/...) can be imported only from the tree that holds
// its internal directory, so neither cmd/ nor internal/ reaches it, and no
// depguard rule is what keeps it out. The module's own root package, were one
// added, is left out too: no rel names it, and a rule's files: list holds a
// directory, which the module root is not; the repository has none today.
//
// This is the class of packages the two properties below close (issue #152, the
// auditor's findings M-A, M-B and H-1).
func modulePackageDirs(t *testing.T) []string {
	t.Helper()

	goreleaser := readRepositoryFile(t, ".goreleaser.yaml")

	targets, err := releasetarget.Read(goreleaser)
	if err != nil {
		t.Fatalf("releasetarget.Read(.goreleaser.yaml): %v", err)
	}

	if len(targets) == 0 {
		t.Fatal("releasetarget.Read(.goreleaser.yaml) returned no target, so this check would run on nothing")
	}

	root := repositoryRoot(t)
	dirs := make(map[string]bool)

	for _, target := range targets {
		pkgs, err := check.ListPackages(t.Context(), root, target, false, []string{"e2e", "integration"}, "./...")
		if err != nil {
			t.Fatalf("ListPackages(%v): %v", target, err)
		}

		for _, pkg := range pkgs {
			if pkg.Standard {
				continue
			}

			rel, ok := strings.CutPrefix(pkg.ImportPath, modulePath+"/")
			if !ok || hiddenByGoInternalRule(rel) {
				continue
			}

			dirs[rel] = true
		}
	}

	// A listing that read nothing would pass both checks below without looking
	// at a package, so the packages the properties exist for are named, one of
	// them under test/: that tree is what the whole-module listing adds, and a
	// listing narrowed back to internal/ and cmd/ would still see the other
	// three.
	for _, want := range []string{
		"internal/config",
		"internal/adapter/source/sourcetest",
		"internal/adapter/target/targettest",
		"test/lint",
	} {
		if !dirs[want] {
			t.Errorf("go list did not list %s/%s, so the checks below read nothing there", modulePath, want)
		}
	}

	if len(dirs) == 0 {
		t.Fatal("go list listed no non-test module package")
	}

	return slices.Sorted(maps.Keys(dirs))
}

// hiddenByGoInternalRule reports whether Go's internal rule keeps every package
// under internal/ and cmd/ from importing rel, a package path relative to the
// module root: whether rel holds an element named internal whose parent is not
// the module root. internal/config is importable from anywhere in the module,
// so it is no exemption; test/internal/yamlread and
// test/architecture/internal/check are importable only from below their own
// internal directory's parent, so no composition root and no driving adapter
// reaches them.
func hiddenByGoInternalRule(rel string) bool {
	for i, element := range strings.Split(rel, "/") {
		if element == "internal" && i > 0 {
			return true
		}
	}

	return false
}

// strictRuleCoverage is one strict depguard rule as the two properties below read
// it: the directories its files: list holds, the directories its negated globs
// take out, whether it holds the test files alone, and its allow list as
// .golangci.yml writes it.
type strictRuleCoverage struct {
	name      string
	dirs      []string
	excludes  []string
	testsOnly bool
	allow     []string
}

// strictRules returns every strict depguard rule of .golangci.yml as
// strictRuleCoverage, in document order: its files: list read with
// check.RuleDirs, and its allow list read with check.DepguardRules as written.
//
// check.AllowList is not used: it refuses the $gostd entry of e2e-is-black-box,
// which is no import path. The properties below only ask whether an entry names
// os, which the written list answers. A strict rule whose files: list has no
// shape check.RuleDirs reads fails here, as TestEveryStrictRuleIsChecked says.
func strictRules(t *testing.T, golangci []byte) []strictRuleCoverage {
	t.Helper()

	rules, err := check.DepguardRules(golangci)
	if err != nil {
		t.Fatalf("DepguardRules(.golangci.yml): %v", err)
	}

	var strict []strictRuleCoverage

	for _, rule := range rules {
		if rule.ListMode != "strict" {
			continue
		}

		files, err := check.RuleDirs(golangci, rule.Name)
		if err != nil {
			t.Fatalf("RuleDirs(.golangci.yml, %q): %v", rule.Name, err)
		}

		strict = append(strict, strictRuleCoverage{
			name:      rule.Name,
			dirs:      files.Dirs,
			excludes:  files.Excludes,
			testsOnly: files.TestsOnly,
			allow:     rule.Allow,
		})
	}

	if len(strict) == 0 {
		t.Fatal("DepguardRules(.golangci.yml) returned no strict rule, so the checks below would run on nothing")
	}

	return strict
}

// holds reports whether the rule holds the package in dir, a slash-separated path
// from the module root: dir or a directory below it is in its files: list, no
// exclusion takes it out, and the rule holds production files, not the test files
// alone.
func (r strictRuleCoverage) holds(dir string) bool {
	if r.testsOnly {
		return false
	}

	under := func(parent string) bool { return dir == parent || strings.HasPrefix(dir, parent+"/") }

	if !slices.ContainsFunc(r.dirs, under) {
		return false
	}

	return !slices.ContainsFunc(r.excludes, under)
}

// allowsOS reports whether the rule's allow list lets a package import os, as
// .golangci.yml writes the entry: exact (os$) and as a prefix (os, which depguard
// reads as every path starting with os) alike, and the $gostd entry, which allows
// every standard package, os among them (issue #167), so a widening is no way
// past the property either.
func (r strictRuleCoverage) allowsOS() bool {
	return slices.Contains(r.allow, "os$") || slices.Contains(r.allow, "os") || r.allowsGostd()
}

// allowsSyscall reports whether the rule's allow list lets a package import
// syscall, read as allowsOS reads os (issue #167): exact (syscall$), as a prefix
// (syscall, which also allows syscall/js but allows syscall all the same) and the
// $gostd entry. An entry for syscall/js alone allows no syscall.
func (r strictRuleCoverage) allowsSyscall() bool {
	return slices.Contains(r.allow, "syscall$") || slices.Contains(r.allow, "syscall") || r.allowsGostd()
}

// allowsGostd reports whether the rule's allow list lets a package import any
// standard package: the $gostd entry, which depguard expands to one prefix entry
// per top-level directory of GOROOT/src. It is read as written and not expanded:
// the written entry is what a reviewer reads, and no other entry of a rule means
// the whole standard library.
func (r strictRuleCoverage) allowsGostd() bool {
	return slices.Contains(r.allow, gostdEntry)
}

// gostdOnlyExempt are the non-test module packages a strict depguard rule may
// hold alone while allowing $gostd, each with the reason the package needs no
// second rule naming its imports. It is empty on purpose: since issue #152 (the
// reviewer's finding B1, the auditor's L-1) every non-test module package a
// composition root can import that a $gostd rule holds is held by a strict allow
// list of its own names as well, so an entry here accepts a package behind a rule
// that allows every standard package, os/exec, syscall and unsafe among them, and
// a reviewer reads the entry as that acceptance.
var gostdOnlyExempt = map[string]string{}

// TestNoPackageIsHeldOnlyByAGostdRule closes the last hole issue #152 opened (the
// reviewer's finding B1 and the auditor's L-1): a strict rule that allows $gostd
// allows every standard package, os/exec, syscall and unsafe among them, so a
// package such a rule holds alone is a place a composition root hands the work to
// and nobody has written down what the package may import. test/e2e was that
// place: its doc.go carries no build tag, so it is a production package
// cmd/shift2eu may import, and e2e-is-black-box, the only rule that held it,
// allows $gostd. An untagged file there that read os.Getenv and started
// exec.Command stayed lint green with every test passing, because no rule refused
// it and go list -deps never lists a package only a _test.go file imports.
//
// The property is a coverage one, read from .golangci.yml and go list, offline and
// without golangci-lint: every non-test module package a composition root can
// import (modulePackageDirs) that a strict rule holds is held by at least one
// holding rule that does not allow $gostd, or it is named in gostdOnlyExempt with
// the reason it may stand behind a rule that allows the whole standard library. A
// package no strict rule holds at all is left to
// TestEveryModulePackageIsHeldByAStrictRule, which names it.
func TestNoPackageIsHeldOnlyByAGostdRule(t *testing.T) {
	t.Parallel()

	golangci := readRepositoryFile(t, ".golangci.yml")
	strict := strictRules(t, golangci)

	for dir, reason := range gostdOnlyExempt {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("gostdOnlyExempt names %s with no reason: say why the package may stand behind a rule that allows $gostd", dir)
		}
	}

	// holders returns the strict rules that hold dir, in document order.
	holders := func(dir string) []strictRuleCoverage {
		return slices.DeleteFunc(slices.Clone(strict), func(r strictRuleCoverage) bool { return !r.holds(dir) })
	}

	checked := 0

	for _, dir := range modulePackageDirs(t) {
		holding := holders(dir)
		if len(holding) == 0 {
			// A package no strict rule holds is
			// TestEveryModulePackageIsHeldByAStrictRule's finding.
			continue
		}

		checked++

		if slices.ContainsFunc(holding, func(r strictRuleCoverage) bool { return !r.allowsGostd() }) {
			continue
		}

		if _, exempt := gostdOnlyExempt[dir]; exempt {
			continue
		}

		names := make([]string, 0, len(holding))
		for _, r := range holding {
			names = append(names, r.name)
		}

		t.Errorf("%s/%s: the strict rules that hold it, %s, all allow $gostd, so no rule says what the package may import: a composition root could hand it work, and an untagged file there could read the environment or start a process with lint green. Hold it to a strict allow list of what it imports today, as test-docs-are-pure holds the test trees' documentation packages, or name it in gostdOnlyExempt with the reason it may stand behind $gostd (issue #152)",
			modulePath, dir, strings.Join(names, ", "))
	}

	if checked == 0 {
		t.Fatal("no non-test module package a composition root can import is held by a strict rule, so this check read nothing")
	}
}

// TestEveryModulePackageIsHeldByAStrictRule closes the delegation gap issue #152
// opened (the auditor's finding M-A): a composition root may import every package
// of the module, so a package no strict depguard rule holds is a place a root
// hands the work to and nobody has written down what it may import. Before this
// test internal/config, internal/adapter/source/sourcetest and
// internal/adapter/target/targettest fell under no rule at all.
//
// The property is read from .golangci.yml, offline and without golangci-lint:
// every non-test module package a composition root can import, listed with go
// list for every release target, is held by at least one strict rule whose
// files: list names its directory (or one above it), whose exclusions do not
// take it out, and which holds production files. The packages Go's internal rule
// keeps out of cmd/ and internal/, and the module's own root package, are left
// out for the reasons modulePackageDirs gives.
func TestEveryModulePackageIsHeldByAStrictRule(t *testing.T) {
	t.Parallel()

	golangci := readRepositoryFile(t, ".golangci.yml")
	strict := strictRules(t, golangci)

	for _, dir := range modulePackageDirs(t) {
		if slices.ContainsFunc(strict, func(r strictRuleCoverage) bool { return r.holds(dir) }) {
			continue
		}

		t.Errorf("%s/%s: no strict depguard rule holds it, so a composition root could hand it work and nothing would say what it may import; add a rule whose files: list names **/%s/** and whose allow list is what the package imports today, and pin it in pinnedDepguardRules (test/architecture/depguardpin_test.go) in the same commit (issue #152)",
			modulePath, dir, dir)
	}
}

// osEnvironmentReads are the os calls that reach the environment or a directory
// an environment variable names, each with the import path forbidigo renders it
// by: every way to read a named variable, the whole environment, a process start
// that takes an environment, and the four lookups that locate a home, config,
// cache or temp directory from an unnamed variable. A package a strict rule
// allows os (a composition root for os.Args, os.Exit and os.Interrupt, the CLI
// and inventoryfile for the file a user names, the test helpers for a golden
// file) may make none of them: the environment is where credentials live
// (CLAUDE.md rule 3), and a root that may not read it may hand the read to a
// package it constructs.
var osEnvironmentReads = []struct{ pkg, name string }{
	{"os", "os.Getenv"},
	{"os", "os.LookupEnv"},
	{"os", "os.ExpandEnv"},
	{"os", "os.Environ"},
	{"os", "os.StartProcess"},
	{"os", "os.UserHomeDir"},
	{"os", "os.UserConfigDir"},
	{"os", "os.UserCacheDir"},
	{"os", "os.TempDir"},
}

// syscallEnvironmentReads are the syscall calls that reach the environment or
// start a process, as osEnvironmentReads are os's (issue #167): every way to read
// a named variable or the whole environment, every process start or replacement,
// every library load whose code would run, and the raw system calls, which can
// make any of those without naming it. A package a strict rule allows syscall
// (today the composition roots, for a SIG* constant and syscall.Signal, and
// test/e2e through $gostd) may make none of them; every name is one the
// composition roots' forbidigo groups composition-root-no-exec and
// composition-root-env refuse. The names are matched as strings against the
// groups' patterns, so a name that exists on one release target only (the
// windows CreateProcess, the linux AllThreadsSyscall) costs nothing on the
// others, and make lint lints every release target.
var syscallEnvironmentReads = []struct{ pkg, name string }{
	{"syscall", "syscall.Getenv"},
	{"syscall", "syscall.GetEnvironmentVariable"},
	{"syscall", "syscall.GetEnvironmentStrings"},
	{"syscall", "syscall.Environ"},
	{"syscall", "syscall.Exec"},
	{"syscall", "syscall.ForkExec"},
	{"syscall", "syscall.StartProcess"},
	{"syscall", "syscall.CreateProcess"},
	{"syscall", "syscall.CreateProcessAsUser"},
	{"syscall", "syscall.LoadDLL"},
	{"syscall", "syscall.MustLoadDLL"},
	{"syscall", "syscall.NewLazyDLL"},
	{"syscall", "syscall.LoadLibrary"},
	{"syscall", "syscall.GetProcAddress"},
	{"syscall", "syscall.Syscall"},
	{"syscall", "syscall.Syscall6"},
	{"syscall", "syscall.Syscall9"},
	{"syscall", "syscall.Syscall12"},
	{"syscall", "syscall.Syscall15"},
	{"syscall", "syscall.Syscall18"},
	{"syscall", "syscall.SyscallN"},
	{"syscall", "syscall.RawSyscall"},
	{"syscall", "syscall.RawSyscall6"},
	{"syscall", "syscall.AllThreadsSyscall"},
	{"syscall", "syscall.AllThreadsSyscall6"},
}

// requiredEnvironmentReads returns the reads a forbidigo group must refuse for a
// package the strict rules in holders hold (issue #167): osEnvironmentReads when
// any holder allows os, syscallEnvironmentReads when any allows syscall, both
// when they do (one holder allowing $gostd among them), and none otherwise.
func requiredEnvironmentReads(holders []strictRuleCoverage) []struct{ pkg, name string } {
	var reads []struct{ pkg, name string }

	if slices.ContainsFunc(holders, strictRuleCoverage.allowsOS) {
		reads = append(reads, osEnvironmentReads...)
	}

	if slices.ContainsFunc(holders, strictRuleCoverage.allowsSyscall) {
		reads = append(reads, syscallEnvironmentReads...)
	}

	return reads
}

// forbidigoPattern is one entry of linters.settings.forbidigo.forbid: its pkg
// (nil when the entry names none, which matches every package) and its pattern,
// compiled, and the tag that starts its msg.
type forbidigoPattern struct {
	tag string
	pkg *regexp.Regexp
	re  *regexp.Regexp
}

// groupScope is one linters.exclusions.rules entry that scopes a forbidigo
// group: the group holds for a file the rule's path matches, or one its
// path-except does not, unless a rule drops it.
type groupScope struct {
	path       *regexp.Regexp
	pathExcept *regexp.Regexp
}

// groupTagOf returns the tag an exclusion rule's text scopes, when the text is
// the whole-group form 'because "<tag>: [^"]*"$', and whether it has that form.
// A narrowed text, which drops one entry's msg, scopes no whole group and is
// left out: TestForbidigoExclusionsDropOneGroup (test/lint) pins that every
// forbidigo exclusion rule has one of the two forms.
func groupTagOf(text string) (string, bool) {
	rest, ok := strings.CutPrefix(text, `because "`)
	if !ok {
		return "", false
	}

	return strings.CutSuffix(rest, `: [^"]*"$`)
}

// forbidigoCoverage reads forbidigo's entries and the group-scoped exclusion
// rules that say where each group holds, from .golangci.yml, offline and without
// golangci-lint: the entries by the tag that starts every msg, and the scopes by
// the tag an exclusion rule's text names. It fails closed on what it does not
// read.
func forbidigoCoverage(t *testing.T, golangci []byte) (map[string][]forbidigoPattern, map[string][]groupScope) {
	t.Helper()

	lines, err := yamlread.SplitLines(golangci)
	if err != nil {
		t.Fatalf("read .golangci.yml: %v", err)
	}

	block, err := yamlread.Walk(lines, "linters", "settings", "forbidigo", "forbid")
	if err != nil {
		t.Fatalf("read forbidigo's entries: %v", err)
	}

	items, err := yamlread.Sequence(block)
	if err != nil {
		t.Fatalf("read forbidigo's entries: %v", err)
	}

	patterns := make(map[string][]forbidigoPattern, len(items))

	for _, it := range items {
		fields, err := yamlread.Mapping(it.Lines)
		if err != nil {
			t.Fatalf("read a forbidigo entry: %v", err)
		}

		var (
			pattern, pkg, msg string
			at                = fmt.Sprintf(".golangci.yml:%d", it.Num)
		)

		for _, field := range fields {
			switch field.Key {
			case "pattern":
				pattern, err = yamlread.Single(field)
			case "pkg":
				pkg, err = yamlread.Single(field)
			case "msg":
				msg, err = yamlread.Single(field)
			default:
				err = fmt.Errorf("%s: forbidigo entry key %s, which this check does not read: want pattern, pkg or msg", at, field.Key)
			}

			if err != nil {
				t.Fatalf("%s: %v", at, err)
			}
		}

		if pattern == "" || msg == "" {
			t.Fatalf("%s: a forbidigo entry without a pattern or a msg", at)
		}

		tag, _, ok := strings.Cut(msg, ": ")
		if !ok {
			t.Fatalf("%s: forbidigo entry msg %q does not start with a group tag; every msg starts with its group's tag", at, msg)
		}

		entry := forbidigoPattern{tag: tag}

		entry.re, err = regexp.Compile(pattern)
		if err != nil {
			t.Fatalf("%s: pattern: %v", at, err)
		}

		if pkg != "" {
			entry.pkg, err = regexp.Compile(pkg)
			if err != nil {
				t.Fatalf("%s: pkg: %v", at, err)
			}
		}

		patterns[tag] = append(patterns[tag], entry)
	}

	if len(patterns) == 0 {
		t.Fatal(".golangci.yml holds no forbidigo entry")
	}

	return patterns, forbidigoScopes(t, lines)
}

// forbidigoScopes reads linters.exclusions.rules: for every rule that names
// forbidigo and whose text drops a whole group's issues, the path or path-except
// that scopes the group the text names. A rule that names another linter only, or
// one whose text is a narrowed exception for one entry, scopes nothing here.
func forbidigoScopes(t *testing.T, lines []yamlread.Line) map[string][]groupScope {
	t.Helper()

	block, err := yamlread.Walk(lines, "linters", "exclusions", "rules")
	if err != nil {
		t.Fatalf("read linters.exclusions.rules: %v", err)
	}

	items, err := yamlread.Sequence(block)
	if err != nil {
		t.Fatalf("read linters.exclusions.rules: %v", err)
	}

	scopes := make(map[string][]groupScope)

	for _, it := range items {
		fields, err := yamlread.Mapping(it.Lines)
		if err != nil {
			t.Fatalf("read an exclusion rule: %v", err)
		}

		var (
			scope       groupScope
			text        string
			namesForbid bool
			at          = fmt.Sprintf(".golangci.yml:%d", it.Num)
		)

		for _, field := range fields {
			switch field.Key {
			case "linters":
				linters, err := yamlread.List(field)
				if err != nil {
					t.Fatalf("%s: %v", at, err)
				}

				namesForbid = len(linters) == 0 || slices.Contains(linters, "forbidigo")
			case "text":
				text, err = yamlread.Single(field)
			case "path":
				var pattern string
				pattern, err = yamlread.Single(field)
				if err == nil {
					scope.path, err = regexp.Compile(pattern)
				}
			case "path-except":
				var pattern string
				pattern, err = yamlread.Single(field)
				if err == nil {
					scope.pathExcept, err = regexp.Compile(pattern)
				}
			default:
				err = fmt.Errorf("%s: exclusion rule key %s, which this check does not read", at, field.Key)
			}

			if err != nil {
				t.Fatalf("%s: %v", at, err)
			}
		}

		if !namesForbid || text == "" {
			continue
		}

		tag, ok := groupTagOf(text)
		if !ok {
			continue
		}

		if (scope.path == nil) == (scope.pathExcept == nil) {
			t.Fatalf("%s: an exclusion rule of group %s must name exactly one of path and path-except", at, tag)
		}

		scopes[tag] = append(scopes[tag], scope)
	}

	return scopes
}

// inScope reports whether the group with tag holds for file, a slash-separated
// path from the module root: golangci-lint drops an issue when a rule's path
// matches it, or when its path-except does not.
func inScope(scopes []groupScope, file string) bool {
	for _, scope := range scopes {
		if scope.path != nil && scope.path.MatchString(file) {
			return false
		}

		if scope.pathExcept != nil && !scope.pathExcept.MatchString(file) {
			return false
		}
	}

	return true
}

// coverageProbeFile is the file name the environment-group property reads the
// forbidigo scopes for, in a directory the rule holds: a production file name,
// not a _test.go one, and one no file of the repository has, so the probe cannot
// collide with a real file a path rule names (issue #152, the reviewer's N2).
const coverageProbeFile = "strictcoverage_probe_production.go"

// TestEveryOsAllowingRuleHasAnEnvironmentGroup closes the second half of the
// delegation gap issue #152 opened (the auditor's finding M-B): a depguard rule
// that allows os lets the package read files, and os also reads and hands over
// the environment, where credentials live (CLAUDE.md rule 3). An import rule
// cannot say which call of os a package makes, so every non-test module package
// a composition root can import (modulePackageDirs) whose strict rule allows os
// must be held by a forbidigo group that refuses every read of
// osEnvironmentReads: a root could otherwise hand the read to a package it
// constructs, with lint green. Since issue #167 the same holds for syscall, which
// reads the environment and starts processes as os does: a package whose strict
// rule allows syscall must be refused every read of syscallEnvironmentReads, and
// one whose rule allows $gostd, every standard package, both lists
// (requiredEnvironmentReads). Before this test
// internal/adapter/source/inventoryfile, which cmd/shift2eu imports, had no such
// group, and the composition roots' groups refused no directory lookup.
//
// The property is read from .golangci.yml, offline and without golangci-lint: the
// packages come from go list for every release target (modulePackageDirs), the
// rules from the depguard settings, and the groups and where they hold from the
// forbidigo settings and linters.exclusions.rules.
func TestEveryOsAllowingRuleHasAnEnvironmentGroup(t *testing.T) {
	t.Parallel()

	golangci := readRepositoryFile(t, ".golangci.yml")
	strict := strictRules(t, golangci)
	patterns, scopes := forbidigoCoverage(t, golangci)

	for _, dir := range modulePackageDirs(t) {
		holders := slices.DeleteFunc(slices.Clone(strict), func(r strictRuleCoverage) bool { return !r.holds(dir) })
		required := requiredEnvironmentReads(holders)
		if len(required) == 0 {
			continue
		}

		file := dir + "/" + coverageProbeFile
		refused := make(map[string]bool)

		for tag := range patterns {
			if !inScope(scopes[tag], file) {
				continue
			}

			for _, read := range required {
				if slices.ContainsFunc(patterns[tag], func(p forbidigoPattern) bool {
					return (p.pkg == nil || p.pkg.MatchString(read.pkg)) && p.re.MatchString(read.name)
				}) {
					refused[read.name] = true
				}
			}
		}

		var missing []string

		for _, read := range required {
			if !refused[read.name] {
				missing = append(missing, read.name)
			}
		}

		if len(missing) != 0 {
			t.Errorf("%s/%s: its strict rule allows os or syscall, but no forbidigo group in scope refuses %s; add a group with an entry refusing os or syscall, or widen the group that holds the package, and a linters.exclusions.rules rule that scopes it, pinned by TestForbidigoExclusionsDropOneGroup (issues #152 and #167, CLAUDE.md rule 3)",
				modulePath, dir, strings.Join(missing, ", "))
		}
	}
}

// TestHiddenByGoInternalRule pins the one exemption modulePackageDirs makes: a
// package path whose internal element has a parent other than the module root is
// unreachable from cmd/ and internal/, and one whose only internal element is the
// module's own is not. The negative cases are the ones the exemption must not
// swallow: internal/config, which cmd/shift2eu imports, a directory that merely
// starts with the word, and test/lint, which the whole-module listing is for.
func TestHiddenByGoInternalRule(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		rel  string
		want bool
	}{
		{name: "the module's own internal tree is importable from cmd", rel: "internal/config", want: false},
		{name: "a package below the module's own internal tree", rel: "internal/adapter/source/inventoryfile", want: false},
		{name: "a test tree package", rel: "test/lint", want: false},
		{name: "a directory whose name only starts with internal", rel: "test/internals/yamlread", want: false},
		{name: "test/internal", rel: "test/internal/yamlread", want: true},
		{name: "a nested internal tree under test", rel: "test/architecture/internal/check", want: true},
		{name: "a package below a nested internal tree", rel: "test/architecture/internal/check/testdata", want: true},
		{name: "a top-level internal tree beside the module's own", rel: "pkg/internal/helper", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := hiddenByGoInternalRule(tc.rel); got != tc.want {
				t.Errorf("hiddenByGoInternalRule(%q) = %t, want %t", tc.rel, got, tc.want)
			}
		})
	}
}

// TestStrictRuleCoverageHolds pins the seam the two properties above share: a rule
// holds a directory its files: list names, not one an exclusion takes out, and not
// a package whose imports only a rule for its test files holds; and a rule allows
// os exactly when its allow list names os, exactly or as a prefix.
func TestStrictRuleCoverageHolds(t *testing.T) {
	t.Parallel()

	rule := strictRuleCoverage{
		name:     "example-is-pure",
		dirs:     []string{"internal/example"},
		excludes: []string{"internal/example/testdata"},
		allow:    []string{"errors$", "os$"},
	}

	for _, tc := range []struct {
		dir  string
		want bool
	}{
		{dir: "internal/example", want: true},
		{dir: "internal/example/sub", want: true},
		{dir: "internal/example/testdata", want: false},
		{dir: "internal/example/testdata/sub", want: false},
		{dir: "internal/other", want: false},
		{dir: "internal/examples", want: false},
	} {
		if got := rule.holds(tc.dir); got != tc.want {
			t.Errorf("holds(%q) = %t, want %t", tc.dir, got, tc.want)
		}
	}

	testFilesOnly := rule
	testFilesOnly.testsOnly = true

	if testFilesOnly.holds("internal/example") {
		t.Error("a rule for the test files alone holds no production package")
	}

	if !rule.allowsOS() {
		t.Error("allowsOS() = false, want true: the allow list names os$")
	}

	prefix := rule
	prefix.allow = []string{"os"}

	if !prefix.allowsOS() {
		t.Error("allowsOS() = false, want true: an entry without $ allows every path starting with os")
	}

	withoutOS := rule
	withoutOS.allow = []string{"errors$", "os/exec$"}

	if withoutOS.allowsOS() {
		t.Error("allowsOS() = true, want false: os/exec is not os")
	}

	// The $gostd entry stands for a prefix entry per top-level directory of
	// GOROOT/src, os among them, so a rule that allows the whole standard
	// library allows os too (issue #167).
	gostd := rule
	gostd.allow = []string{gostdEntry}

	if !gostd.allowsOS() {
		t.Error("allowsOS() = false, want true: the $gostd entry allows every standard package, os among them (issue #167)")
	}
}

// TestStrictRuleCoverageAllowsSyscall pins allowsSyscall, the syscall half of the
// seam the properties above read (issue #167): syscall reaches the environment and
// starts processes as os does (syscall.Getenv, Environ, Exec, ForkExec,
// StartProcess), so a rule that allows it is a place to read a credential from, and
// the property must see it. The allow list is read as .golangci.yml writes it:
// exact (syscall$) and as a prefix (syscall, which depguard reads as every path
// starting with it) alike, and the $gostd entry counts as allowing it. The
// negative cases are the ones the reading must not swallow: syscall/js is another
// package, golang.org/x/sys/unix is no standard package at all, and os is no
// syscall.
func TestStrictRuleCoverageAllowsSyscall(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		allow []string
		want  bool
	}{
		{name: "an exact syscall entry", allow: []string{"syscall$"}, want: true},
		{name: "a prefix syscall entry", allow: []string{"syscall"}, want: true},
		{name: "the $gostd entry", allow: []string{gostdEntry}, want: true},
		{name: "syscall/js is another package", allow: []string{"syscall/js$"}, want: false},
		{name: "golang.org/x/sys/unix is no standard package", allow: []string{"golang.org/x/sys/unix$"}, want: false},
		{name: "os is no syscall", allow: []string{"os$"}, want: false},
		{name: "an empty allow list", allow: nil, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rule := strictRuleCoverage{name: "example-is-pure", allow: tc.allow}

			if got := rule.allowsSyscall(); got != tc.want {
				t.Errorf("allowsSyscall() with allow %v = %t, want %t (issue #167)", tc.allow, got, tc.want)
			}
		})
	}
}

// environmentReadNames returns the names of the reads in reads, sorted, so a test
// can compare what a forbidigo group must refuse as one list and the order neither
// the lists nor requiredEnvironmentReads builds them in decides the result.
func environmentReadNames(reads []struct{ pkg, name string }) []string {
	names := make([]string, 0, len(reads))

	for _, read := range reads {
		names = append(names, read.name)
	}

	slices.Sort(names)

	return names
}

// TestRequiredEnvironmentReads pins requiredEnvironmentReads (issue #167): the
// reads a forbidigo group must refuse for a package the given strict rules hold,
// read from the rules' allow lists. A rule that names neither os nor syscall
// requires nothing, one that names os requires the os reads, one that names
// syscall requires the syscall reads, either order of holder gives both, and the
// $gostd entry requires both because it allows every standard package, syscall
// among them. That last case is the reason the property must read $gostd at all: a
// rule that allows the whole standard library is the widest way to reach the
// environment, and it is the one the earlier os-only reading passed unchecked.
func TestRequiredEnvironmentReads(t *testing.T) {
	t.Parallel()

	osNames := environmentReadNames(osEnvironmentReads)
	syscallNames := environmentReadNames(syscallEnvironmentReads)
	bothNames := environmentReadNames(slices.Concat(osEnvironmentReads, syscallEnvironmentReads))

	allowOS := strictRuleCoverage{name: "example-allows-os", allow: []string{"errors$", "os$"}}
	allowSyscall := strictRuleCoverage{name: "example-allows-syscall", allow: []string{"syscall$"}}

	for _, tc := range []struct {
		name    string
		holders []strictRuleCoverage
		want    []string
	}{
		{
			name:    "no holder",
			holders: nil,
			want:    nil,
		},
		{
			name:    "a holder that allows neither os nor syscall",
			holders: []strictRuleCoverage{{name: "example-is-pure", allow: []string{"errors$"}}},
			want:    nil,
		},
		{
			name:    "a holder that allows os",
			holders: []strictRuleCoverage{allowOS},
			want:    osNames,
		},
		{
			name:    "a holder that allows syscall",
			holders: []strictRuleCoverage{allowSyscall},
			want:    syscallNames,
		},
		{
			name:    "one holder that allows os and another that allows syscall",
			holders: []strictRuleCoverage{allowOS, allowSyscall},
			want:    bothNames,
		},
		{
			name:    "a holder that allows $gostd",
			holders: []strictRuleCoverage{{name: "example-is-gostd", allow: []string{gostdEntry}}},
			want:    bothNames,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := environmentReadNames(requiredEnvironmentReads(tc.holders))

			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("requiredEnvironmentReads() mismatch (-want +got):\n%s (issue #167)", diff)
			}
		})
	}

	// A syscall reads list that drifted from the calls the property exists for
	// would let a syscall-allowing rule past unchecked, so the calls the issue
	// names are pinned here too (issue #167).
	wantSyscall := []string{
		"syscall.AllThreadsSyscall",
		"syscall.AllThreadsSyscall6",
		"syscall.CreateProcess",
		"syscall.CreateProcessAsUser",
		"syscall.Exec",
		"syscall.Environ",
		"syscall.ForkExec",
		"syscall.GetEnvironmentStrings",
		"syscall.GetEnvironmentVariable",
		"syscall.Getenv",
		"syscall.GetProcAddress",
		"syscall.LoadDLL",
		"syscall.LoadLibrary",
		"syscall.MustLoadDLL",
		"syscall.NewLazyDLL",
		"syscall.RawSyscall",
		"syscall.RawSyscall6",
		"syscall.StartProcess",
		"syscall.Syscall",
		"syscall.Syscall12",
		"syscall.Syscall15",
		"syscall.Syscall18",
		"syscall.Syscall6",
		"syscall.Syscall9",
		"syscall.SyscallN",
	}

	for _, read := range syscallEnvironmentReads {
		if read.pkg != "syscall" {
			t.Errorf("syscallEnvironmentReads holds %s.%s, want every read in package syscall (issue #167)", read.pkg, read.name)
		}
	}

	var missing []string

	for _, want := range wantSyscall {
		if !slices.ContainsFunc(syscallEnvironmentReads, func(read struct{ pkg, name string }) bool {
			return read.name == want
		}) {
			missing = append(missing, want)
		}
	}

	if len(missing) != 0 {
		t.Errorf("syscallEnvironmentReads names no %s, so a rule that allows syscall reaches the environment past the property (issue #167)", strings.Join(missing, ", "))
	}
}
