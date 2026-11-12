package check

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/test/internal/releasetarget"
)

// Module is the module a package belongs to, as go list reports it.
type Module struct {
	Path string
	Main bool
}

// Package is one package as `go list -json` reports it, cut down to the fields
// the check reads.
type Package struct {
	ImportPath string
	Dir        string
	Standard   bool
	Module     *Module
	Imports    []string

	// The imports of the package's _test.go files: those of package p
	// (TestImports) and those of the external test package p_test
	// (XTestImports), for the files the build tags select. go list -deps
	// fills them for the packages its patterns match; a package only a test
	// imports is not itself listed.
	TestImports  []string
	XTestImports []string

	// The sources other than Go files that go list reports: what they call,
	// no import rule sees.
	CgoFiles     []string
	CFiles       []string
	CXXFiles     []string
	HFiles       []string
	FFiles       []string
	MFiles       []string
	SFiles       []string
	SwigFiles    []string
	SwigCXXFiles []string
	SysoFiles    []string
}

// listedPackage is a package as go list prints it, with the error fields that
// ListPackages refuses.
type listedPackage struct {
	Package

	Incomplete bool
	Error      *listError
	DepsErrors []*listError
}

// listError is an error go list reports for a package.
type listError struct {
	Err string
}

// Rule holds the packages in one directory of the module, and every directory
// below it but those it excludes, to an allow list of imports.
type Rule struct {
	// Name is the depguard rule the allow list is read from, named in every
	// message the rule earns.
	Name string
	// Dir is the directory, slash-separated and relative to the module root.
	Dir string
	// Excludes are directories, written like Dir, that the rule does not
	// hold, with every directory below them: the negated globs of its files:
	// list.
	Excludes []string
	// Tests is set when the rule holds test files too: their imports
	// (TestImports and XTestImports) are held to the allow list as well.
	Tests bool
	// Allow are the entries the imports are held to, each exact or a prefix
	// as depguard reads it. An import is tested against the entry that sorts
	// nearest before it only, as depguard does (allowedBy).
	Allow []AllowEntry
}

// inheritedEnv are the variables go list takes from the environment the check
// runs in, each because the go command needs it on Linux, macOS or Windows to
// find its toolchain, its caches or a temporary directory. Every other
// variable is left out, so none the check does not know can change the
// listing: not GOFLAGS, GOEXPERIMENT, GOPRIVATE or GONOPROXY (which make
// GOPROXY=off fetch directly), a micro-architecture level such as GOAMD64,
// GOFIPS140, nor one a later Go starts to read. SystemRoot, which Windows
// programs need, is not listed: os/exec adds it on Windows when it is missing.
var inheritedEnv = []string{
	// The go command looks up a toolchain on PATH when GOTOOLCHAIN asks it to
	// (go1.N+path); go test puts the toolchain the test runs under first on it.
	"PATH",
	// The toolchain the test runs under: go test's own toolchain switch sets
	// it, so go list reads the same standard library.
	"GOROOT",
	// The default GOPATH, and with it the module cache, is under the home
	// directory: HOME on Linux and macOS, USERPROFILE on Windows. On macOS the
	// default build cache is too.
	"HOME",
	"USERPROFILE",
	// The default build cache on Linux (XDG_CACHE_HOME, else under HOME) and on
	// Windows (LOCALAPPDATA).
	"XDG_CACHE_HOME",
	"LOCALAPPDATA",
	// Where a developer or CI put the caches instead of the defaults. With
	// GOPROXY=off the listing can use only the modules and toolchains already
	// in the module cache, so it must find the one they fill.
	"GOPATH",
	"GOMODCACHE",
	"GOCACHE",
	// Which toolchain runs the listing. It is not pinned to local, which would
	// break a host whose go is older than go.mod; with GOPROXY=off a switch can
	// use only a toolchain already in the module cache.
	"GOTOOLCHAIN",
	// The temporary directory: TMPDIR on Linux and macOS (a per-user one on
	// macOS), TEMP and TMP on Windows.
	"TMPDIR",
	"TEMP",
	"TMP",
}

// ListPackages runs `go list -deps` in dir for the patterns, built for target
// with cgo on or off and with the build tags, and returns every package the
// patterns match and every package they depend on, in go list's order.
//
// go list prints only the fields Package and the error fields decode
// (-json=<fields>): the full -json computes Stale, which runs the C compiler
// for cgo packages, compile -V=full and git.
//
// The command does not get the inherited environment. It gets the variables of
// inheritedEnv that are set, PWD set to dir as os/exec would set it, and the
// settings the check fixes: GOOS, GOARCH and CGO_ENABLED for the build;
// GOENV=off, so a go env file (go env -w) adds nothing, which an empty variable
// would not prevent; GOFLAGS=-mod=readonly, so no tag and no mode comes from
// elsewhere; GOPROXY=off, so the listing never touches the network; and
// GOWORK=off, so a stray go.work cannot change the listing (the release's go
// command would read one: StrayReleaseFiles refuses one git could commit). GOEXPERIMENT and
// the micro-architecture levels are left unset, so the toolchain's defaults
// pick the files their build tags select, the levels goreleaser builds amd64
// (v1) and arm64 (v8.0) at. -tags is passed only when tags is not empty.
//
// It returns an error when the command fails, with its standard error, or
// when any package it lists is incomplete or has an error of its own or in a
// dependency.
func ListPackages(ctx context.Context, dir string, target releasetarget.Target, cgo bool, tags []string, patterns ...string) ([]Package, error) {
	args := []string{"list", "-deps"}
	args = append(args, "-json="+strings.Join(listFields(reflect.TypeFor[listedPackage]()), ","))

	if len(tags) != 0 {
		args = append(args, "-tags="+strings.Join(tags, ","))
	}

	args = append(args, "--")
	args = append(args, patterns...)

	env, err := listEnv(dir, target, cgo)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return decodePackages(&stdout)
}

// listEnv returns the environment go list runs with in dir: the variables of
// inheritedEnv that are set, PWD, and the settings the check fixes.
func listEnv(dir string, target releasetarget.Target, cgo bool) ([]string, error) {
	env := make([]string, 0, len(inheritedEnv)+8)

	// os.LookupEnv ignores case on Windows, where PATH is often spelt Path.
	for _, name := range inheritedEnv {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}

	// os/exec sets PWD to the command's directory only when it builds the
	// environment itself, and only where PWD has a meaning. The go command
	// takes the working directory from PWD when it names the same directory,
	// so the paths it reports keep the form dir has, symlinks included, rather
	// than whatever PWD the check inherited.
	if runtime.GOOS != "windows" && runtime.GOOS != "plan9" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", dir, err)
		}

		env = append(env, "PWD="+abs)
	}

	cgoEnabled := "0"
	if cgo {
		cgoEnabled = "1"
	}

	return append(env,
		"GOOS="+target.GOOS,
		"GOARCH="+target.GOARCH,
		"CGO_ENABLED="+cgoEnabled,
		"GOENV=off",
		"GOFLAGS=-mod=readonly",
		"GOPROXY=off",
		"GOWORK=off",
	), nil
}

// listFields returns the names of the fields of the struct type t, with those
// of an embedded struct in its place: the fields go list must print for t to
// decode them.
func listFields(t reflect.Type) []string {
	var names []string

	for i := range t.NumField() {
		field := t.Field(i)
		if field.Anonymous {
			names = append(names, listFields(field.Type)...)

			continue
		}

		names = append(names, field.Name)
	}

	return names
}

// decodePackages reads the JSON objects go list prints one after another, and
// refuses a package that is incomplete or has an error.
func decodePackages(r io.Reader) ([]Package, error) {
	var pkgs []Package

	dec := json.NewDecoder(r)

	for {
		var p listedPackage

		err := dec.Decode(&p)
		if errors.Is(err, io.EOF) {
			return pkgs, nil
		}

		if err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}

		switch {
		case p.Error != nil:
			return nil, fmt.Errorf("go list: %s: %s", p.ImportPath, p.Error.Err)
		case len(p.DepsErrors) != 0:
			return nil, fmt.Errorf("go list: %s: a dependency has an error: %s", p.ImportPath, p.DepsErrors[0].Err)
		case p.Incomplete:
			return nil, fmt.Errorf("go list: %s is incomplete", p.ImportPath)
		}

		pkgs = append(pkgs, p.Package)
	}
}

// Packages checks every package of pkgs outside the standard library against
// the rules, and returns one message per violation, sorted, each starting with
// the package's import path and naming the offending file or import. A package
// must pass Sources and also:
//
//   - have an import path under the directory of at least one rule;
//   - import nothing the allow list of any rule whose directory covers it does
//     not hold. Every covering rule applies, not only the one with the longest
//     directory, as depguard holds a file to every rule whose globs match it;
//     each refusing rule earns its own message, naming the rule. A rule that
//     holds test files holds the imports of the package's _test.go files too.
//
// A package under a directory a rule excludes is not held by that rule: it is
// checked against the rules that do hold it, and when none does, it is still
// placed (module, sources, directory) but neither its imports nor its being
// outside every directory are reported, since the rule's files: list names
// its directory, only to take it out.
//
// Unlike Sources, it flags a package outside the module path too: the
// patterns it is given are the layers, so every package they reach outside
// the standard library must be one of them. The exception is third-party code
// a strict allow list lets in (allowedThirdParty): a package outside the
// module path that in-module packages import directly only where a strict
// rule holds the import and every rule holding it allows it (a cloud SDK
// package under a prefix entry of the SDK-only rule), and every package such
// a package imports, directly or not, that no in-module package imports
// directly (smithy-go below the AWS SDK, which no entry names). depguard lints
// no file of a dependency, so none of these packages breaks a rule, and they
// are not checked at all: not their module, not their sources, which in a
// real SDK may hold assembly. Any other package outside the module that the
// layers reach is still reported.
//
// The exemption follows only the strict allow lists in rules. The lax
// deny rules of .golangci.yml, such as go-cmp-only-in-tests and
// cloud-sdks-only-in-provider-adapters, cover these packages too and are held
// by depguard in make lint alone: a non-test go-cmp import in a layer, or an
// SDK import in a new helper package beside the provider adapters, passes
// this check where depguard refuses it. Nor does this check speak for the
// release binary as a whole: the shared layer listing's patterns are the
// layers, not cmd/eusov, and Sources skips every package outside the module.
// A third-party import in internal/cli, whose rule allows $gostd and is
// skipped here, is refused by depguard alone: that strict rule allows only
// $gostd and named module packages. One in cmd/eusov is refused by the strict
// rule composition-root (issue #101): the architecture tests pass this
// function that rule and the packages of cmd/eusov alone, since the composition
// root's listing also reaches internal/cli and internal/version, which no rule
// it reads holds.
func Packages(root, module string, rules []Rule, pkgs []Package) []string {
	m := newModuleRoot(root, module)
	rules = sortedAllowLists(rules)
	exempt := allowedThirdParty(m, rules, pkgs)

	var messages []string

	for _, pkg := range pkgs {
		if pkg.Standard {
			continue
		}

		rel, inModule := m.rel(pkg.ImportPath)
		if !inModule && exempt[pkg.ImportPath] {
			continue
		}

		c := packageCheck{pkg: pkg}

		var (
			covering []Rule
			excluded bool
		)

		if inModule {
			covering, excluded = rulesFor(rules, rel)
		}

		if len(covering) != 0 || excluded {
			c.placement(m, rel)

			for _, rule := range covering {
				c.imports(rule)
			}
		} else {
			c.module(module)
			c.sources()
			c.report("is outside every directory of %s the check holds to an allow list", module)
		}

		messages = append(messages, c.messages...)
	}

	slices.Sort(messages)

	return messages
}

// allowedThirdParty returns the import paths of the packages of pkgs outside
// the module path that an allow list lets in, followed through the import
// graph.
//
// The exemption starts at the third-party paths that in-module packages import
// directly. Such a path is exempt only when every in-module package that
// imports it is held by a strict rule and every rule holding that import
// allows it. The path is refused when any importer is held by no rule (one
// outside every directory, or one a rule excludes), or when a holding rule
// refuses the import. A refused path is never exempt, even when an exempt
// package also imports it. Otherwise one in-module package no rule holds could
// import smithy-go directly because the SDK's closure lets it in for the
// adapter. From the exempt paths the exemption follows every package they
// import that no in-module package imports directly: depguard lints no file of
// a dependency.
//
// A test file's import counts as held only by the covering rules that hold
// tests (Tests), as depguard lints a test file only under a rule whose files:
// list matches it. The check cannot observe that today: go list -deps without
// -test never lists a package that only a test imports. It is kept for parity
// with depguard and with packageCheck.imports.
//
// refused is keyed by import path, not by importer: a test-only third-party
// import, in a package no rule holding tests covers, refuses that same path
// for production code too, should the SDK closure reach it. That fails
// closed, and cannot happen today (go.mod requires only go-cmp), but the
// message Packages then gives the path
// ("is outside every directory ... the check holds to an allow list")
// would mislead: the path was refused for a test import, not a production
// one.
func allowedThirdParty(m moduleRoot, rules []Rule, pkgs []Package) map[string]bool {
	byPath := make(map[string]Package, len(pkgs))
	for _, pkg := range pkgs {
		byPath[pkg.ImportPath] = pkg
	}

	seeds := make(map[string]bool)
	refused := make(map[string]bool)

	// judge records one direct import of a third-party path: allowed when the
	// rules holding it all allow it, refused when none holds it or one
	// refuses it.
	judge := func(holding []Rule, imp string) {
		if _, inModule := m.rel(imp); inModule {
			return
		}

		if len(holding) == 0 || slices.ContainsFunc(holding, func(r Rule) bool { return !allowedBy(r.Allow, imp) }) {
			refused[imp] = true

			return
		}

		seeds[imp] = true
	}

	for _, pkg := range pkgs {
		if pkg.Standard {
			continue
		}

		rel, inModule := m.rel(pkg.ImportPath)
		if !inModule {
			continue
		}

		covering, _ := rulesFor(rules, rel)
		testing := slices.DeleteFunc(slices.Clone(covering), func(r Rule) bool { return !r.Tests })

		for _, imp := range pkg.Imports {
			judge(covering, imp)
		}

		for _, imp := range slices.Concat(pkg.TestImports, pkg.XTestImports) {
			judge(testing, imp)
		}
	}

	exempt := make(map[string]bool)

	var queue []string

	reach := func(imp string) {
		if _, inModule := m.rel(imp); inModule || exempt[imp] || refused[imp] {
			return
		}

		exempt[imp] = true
		queue = append(queue, imp)
	}

	// The order the seeds are reached in does not change the set returned.
	for imp := range seeds {
		reach(imp)
	}

	for len(queue) != 0 {
		imp := queue[0]
		queue = queue[1:]

		for _, dep := range byPath[imp].Imports {
			reach(dep)
		}
	}

	return exempt
}

// Sources checks every package of pkgs whose import path is module or under
// it, and returns one message per violation, sorted, each starting with the
// package's import path and naming the offending file. A package must:
//
//   - belong to the main module, whose path is module, so a nested module
//     under its path is flagged;
//   - sit in the directory its import path names: its directory with symlinks
//     resolved must be root with symlinks resolved, joined with that path, so
//     a symlinked package directory, whose files lint checks under another
//     path, is flagged;
//   - hold no source but Go files: no cgo, C, C++, header, Fortran,
//     Objective-C, assembly, SWIG or syso file, whose calls no import rule
//     sees.
//
// A directory that cannot be resolved is itself a violation, not a reason to
// skip the check. Imports are not checked. Packages of the standard library and
// those outside the module path (third-party code) are skipped: their
// importers' imports are checked by Packages where a strict rule exists.
func Sources(root, module string, pkgs []Package) []string {
	m := newModuleRoot(root, module)

	var messages []string

	for _, pkg := range pkgs {
		if pkg.Standard {
			continue
		}

		rel, inModule := m.rel(pkg.ImportPath)
		if !inModule {
			continue
		}

		c := packageCheck{pkg: pkg}
		c.placement(m, rel)

		messages = append(messages, c.messages...)
	}

	slices.Sort(messages)

	return messages
}

// moduleRoot is the module the checks hold packages to: its path, and its
// root directory as given and with symlinks resolved.
type moduleRoot struct {
	path       string
	dir        string
	realDir    string
	resolveErr error
}

// newModuleRoot resolves the module root dir once for every package checked.
func newModuleRoot(dir, path string) moduleRoot {
	realDir, err := filepath.EvalSymlinks(dir)

	return moduleRoot{path: path, dir: dir, realDir: realDir, resolveErr: err}
}

// rel returns importPath relative to the module path, empty for the module's
// root package, and whether importPath is the module path or under it.
func (m moduleRoot) rel(importPath string) (string, bool) {
	if importPath == m.path {
		return "", true
	}

	return strings.CutPrefix(importPath, m.path+"/")
}

// packageCheck collects the violations of one package.
type packageCheck struct {
	pkg      Package
	messages []string
}

// report adds a violation, prefixed with the package's import path.
func (c *packageCheck) report(format string, args ...any) {
	c.messages = append(c.messages, c.pkg.ImportPath+": "+fmt.Sprintf(format, args...))
}

// placement flags what Sources and Packages both refuse: a package outside the
// main module, a source file that is not a Go file, and a directory that is
// not rel under the module root.
func (c *packageCheck) placement(m moduleRoot, rel string) {
	c.module(m.path)
	c.sources()
	c.directory(m, rel)
}

// module flags a package outside the main module.
func (c *packageCheck) module(module string) {
	switch m := c.pkg.Module; {
	case m == nil:
		c.report("belongs to no module, want the main module %s", module)
	case !m.Main || m.Path != module:
		c.report("belongs to module %s, want the main module %s", m.Path, module)
	}
}

// sources flags every source file that is not a Go file.
func (c *packageCheck) sources() {
	for _, kind := range []struct {
		field string
		files []string
	}{
		{field: "CgoFiles", files: c.pkg.CgoFiles},
		{field: "CFiles", files: c.pkg.CFiles},
		{field: "CXXFiles", files: c.pkg.CXXFiles},
		{field: "HFiles", files: c.pkg.HFiles},
		{field: "FFiles", files: c.pkg.FFiles},
		{field: "MFiles", files: c.pkg.MFiles},
		{field: "SFiles", files: c.pkg.SFiles},
		{field: "SwigFiles", files: c.pkg.SwigFiles},
		{field: "SwigCXXFiles", files: c.pkg.SwigCXXFiles},
		{field: "SysoFiles", files: c.pkg.SysoFiles},
	} {
		for _, file := range kind.files {
			c.report("%s holds %s, whose calls no import rule sees", kind.field, file)
		}
	}
}

// directory flags a package whose directory, symlinks resolved, is not rel,
// the directory its import path names, under the module root.
func (c *packageCheck) directory(m moduleRoot, rel string) {
	if m.resolveErr != nil {
		c.report("cannot resolve the module root %s: %v", m.dir, m.resolveErr)

		return
	}

	want := filepath.Join(m.realDir, filepath.FromSlash(rel))

	got, err := filepath.EvalSymlinks(c.pkg.Dir)

	switch {
	case c.pkg.Dir == "":
		c.report("has no directory, want %s", want)
	case err != nil:
		c.report("cannot resolve its directory %s: %v", c.pkg.Dir, err)
	case got != want:
		c.report("directory %s resolves to %s, want %s: lint checks its files under another path", c.pkg.Dir, got, want)
	}
}

// imports flags every import the rule's allow list does not hold: the
// package's own and, when the rule holds test files, its test files'. An
// import named by more than one of them earns one message.
func (c *packageCheck) imports(rule Rule) {
	holder := "the allow list for " + rule.Dir
	if rule.Name != "" {
		holder = "the allow list of depguard rule " + rule.Name + " for " + rule.Dir
	}

	type importGroup struct {
		imports []string
		where   string
	}

	groups := []importGroup{{imports: c.pkg.Imports}}

	if rule.Tests {
		groups = append(groups,
			importGroup{imports: c.pkg.TestImports, where: " in a test file"},
			importGroup{imports: c.pkg.XTestImports, where: " in an external test file"},
		)
	}

	seen := make(map[string]bool)

	for _, g := range groups {
		for _, imp := range g.imports {
			if seen[imp] {
				continue
			}

			seen[imp] = true

			if !allowedBy(rule.Allow, imp) {
				c.report("imports %s%s, which %s does not hold", imp, g.where, holder)
			}
		}
	}
}

// rulesFor returns every rule whose directory holds rel, a path relative to
// the module, in the order of rules, and whether a rule whose directory covers
// rel takes it out with one of its exclusions.
func rulesFor(rules []Rule, rel string) ([]Rule, bool) {
	var (
		covering []Rule
		excluded bool
	)

	for _, rule := range rules {
		if !under(rel, rule.Dir) {
			continue
		}

		if slices.ContainsFunc(rule.Excludes, func(dir string) bool { return under(rel, dir) }) {
			excluded = true

			continue
		}

		covering = append(covering, rule)
	}

	return covering, excluded
}

// under reports whether rel is dir or a directory below it.
func under(rel, dir string) bool {
	return rel == dir || strings.HasPrefix(rel, dir+"/")
}
