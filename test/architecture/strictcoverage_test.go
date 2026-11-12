package architecture_test

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
	"github.com/timothymamo/shift2eu/test/internal/releasetarget"
	"github.com/timothymamo/shift2eu/test/internal/yamlread"
)

// modulePackageDirs returns the directory, relative to the module root, of every
// module package under internal/ and cmd/: the layers and the packages beside
// them that a composition root could hand the work to. It lists them with go
// list for every release target .goreleaser.yaml builds, with the e2e and
// integration files on as make lint loads them, so a package that only builds on
// one platform is listed too.
//
// The test/ directories are left out on purpose, because a composition root
// cannot import what they hold: a package there either has _test.go files only,
// so there is no importable package at all, or sits under a directory named
// internal (test/internal/..., test/architecture/internal/...), which Go's
// internal rule keeps every package outside test/ from importing. The roots'
// allow lists name the module path as a prefix entry, so that rule, not a
// depguard rule, is what keeps the test trees out. internal/ and cmd/ are where a
// package a root can reach and delegate to lives, which is the class these
// properties close (issue #152, the auditor's findings M-A and M-B).
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
		pkgs, err := check.ListPackages(t.Context(), root, target, false, []string{"e2e", "integration"}, "./internal/...", "./cmd/...")
		if err != nil {
			t.Fatalf("ListPackages(%v): %v", target, err)
		}

		for _, pkg := range pkgs {
			if pkg.Standard {
				continue
			}

			rel, ok := strings.CutPrefix(pkg.ImportPath, modulePath+"/")
			if !ok || (rel != "internal" && !strings.HasPrefix(rel, "internal/") && rel != "cmd" && !strings.HasPrefix(rel, "cmd/")) {
				continue
			}

			dirs[rel] = true
		}
	}

	// A listing that read nothing would pass both checks below without looking
	// at a package, so the packages the properties exist for are named.
	for _, want := range []string{
		"internal/config",
		"internal/adapter/source/sourcetest",
		"internal/adapter/target/targettest",
	} {
		if !dirs[want] {
			t.Errorf("go list did not list %s/%s, so the checks below read nothing there", modulePath, want)
		}
	}

	if len(dirs) == 0 {
		t.Fatal("go list listed no module package under internal/ or cmd/")
	}

	return slices.Sorted(maps.Keys(dirs))
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
// reads as every path starting with os) alike, so a widening is no way past the
// property either.
func (r strictRuleCoverage) allowsOS() bool {
	return slices.Contains(r.allow, "os$") || slices.Contains(r.allow, "os")
}

// TestEveryModulePackageIsHeldByAStrictRule closes the delegation gap issue #152
// opened (the auditor's finding M-A): a composition root may import every package
// of the module, so a package no strict depguard rule holds is a place a root
// hands the work to and nobody has written down what it may import. Before this
// test internal/config, internal/adapter/source/sourcetest and
// internal/adapter/target/targettest fell under no rule at all.
//
// The property is read from .golangci.yml, offline and without golangci-lint:
// every module package under internal/ and cmd/, listed with go list for every
// release target, is held by at least one strict rule whose files: list names its
// directory (or one above it), whose exclusions do not take it out, and which
// holds production files. The test/ trees are left out for the reason
// modulePackageDirs gives.
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

// TestEveryOsAllowingRuleHasAnEnvironmentGroup closes the second half of the
// delegation gap issue #152 opened (the auditor's finding M-B): a depguard rule
// that allows os lets the package read files, and os also reads and hands over
// the environment, where credentials live (CLAUDE.md rule 3). An import rule
// cannot say which call of os a package makes, so every module package under
// internal/ and cmd/ whose strict rule allows os must be held by a forbidigo group
// that refuses every read of osEnvironmentReads: a root could otherwise hand the
// read to a package it constructs, with lint green. Before this test
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
		if !slices.ContainsFunc(holders, strictRuleCoverage.allowsOS) {
			continue
		}

		file := dir + "/probe.go"
		refused := make(map[string]bool)

		for tag := range patterns {
			if !inScope(scopes[tag], file) {
				continue
			}

			for _, read := range osEnvironmentReads {
				if slices.ContainsFunc(patterns[tag], func(p forbidigoPattern) bool {
					return (p.pkg == nil || p.pkg.MatchString(read.pkg)) && p.re.MatchString(read.name)
				}) {
					refused[read.name] = true
				}
			}
		}

		var missing []string

		for _, read := range osEnvironmentReads {
			if !refused[read.name] {
				missing = append(missing, read.name)
			}
		}

		if len(missing) != 0 {
			t.Errorf("%s/%s: its strict rule allows os, but no forbidigo group in scope refuses %s; add a group with an entry refusing os, or widen the group that holds the package, and a linters.exclusions.rules rule that scopes it, pinned by TestForbidigoExclusionsDropOneGroup (issue #152, CLAUDE.md rule 3)",
				modulePath, dir, strings.Join(missing, ", "))
		}
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
}
