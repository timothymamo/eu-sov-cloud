package check_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// testModule is the module path the fake packages claim.
const testModule = "example.com/m"

// TestPackages pins the per-package checks: a package of the main module,
// its import path under a directory a rule covers, its directory the real one
// that path names, no cgo or assembly sources, and no import outside the allow
// list of any rule that covers it. Each case breaks one of them and nothing
// else, so the message it expects is the one that condition earns.
func TestPackages(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// The check compares a package's directory with the one its import path
	// names, so every directory a case's package sits in must exist.
	for _, dir := range []string{
		"internal/domain",
		"internal/domain/text",
		"internal/domain/x",
		"internal/domain/_x",
		"internal/domain/testdata/y",
		"internal/adapter/x",
		"internal/outside",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o700); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}

	// The issue's symlink probe: a package directory that is a symlink to a
	// directory outside the layer, so the package's files are never linted
	// under the path its import claims.
	if err := os.Symlink(
		filepath.Join(root, "internal", "outside"),
		filepath.Join(root, "internal", "domain", "zzlink"),
	); err != nil {
		t.Fatalf("create the symlink probe: %v", err)
	}

	mainModule := &check.Module{Path: testModule, Main: true}

	// mainPkg is a package of the main module in dir, with no imports and no
	// source files: a case sets the one field it breaks on the copy it gets.
	mainPkg := func(dir string) check.Package {
		return check.Package{
			ImportPath: testModule + "/" + dir,
			Dir:        filepath.Join(root, filepath.FromSlash(dir)),
			Module:     mainModule,
		}
	}

	// domainRules hold the domain packages to three imports, so an import of
	// anything else is refused.
	domainRules := []check.Rule{{
		Name:  "domain-is-pure",
		Dir:   "internal/domain",
		Allow: []string{"errors", "fmt", testModule + "/internal/domain/text"},
	}}

	clean := mainPkg("internal/domain")
	clean.Imports = []string{"errors", "fmt", testModule + "/internal/domain/text"}

	standardLibrary := check.Package{
		ImportPath: "fmt",
		Standard:   true,
		Imports:    []string{"errors", "os", "unsafe"},
	}

	nestedModule := mainPkg("internal/domain")
	nestedModule.Module = &check.Module{Path: testModule + "/internal/domain"}
	nestedModule.Imports = []string{"errors"}

	noModule := mainPkg("internal/domain")
	noModule.Module = nil

	outside := mainPkg("internal/adapter/x")

	assembly := mainPkg("internal/domain")
	assembly.SFiles = []string{"zz_arm64.s"}

	cgoFiles := mainPkg("internal/domain")
	cgoFiles.CgoFiles = []string{"zz_cgo.go"}

	cFiles := mainPkg("internal/domain")
	cFiles.CFiles = []string{"zz.c"}

	sysoFiles := mainPkg("internal/domain")
	sysoFiles.SysoFiles = []string{"zz.syso"}

	importsOS := mainPkg("internal/domain")
	importsOS.Imports = []string{"os"}

	symlinked := mainPkg("internal/domain/zzlink")

	underscore := mainPkg("internal/domain/_x")
	underscore.Imports = []string{"os"}

	testdataDir := mainPkg("internal/domain/testdata/y")
	testdataDir.Imports = []string{"os"}

	// A package go list could not place: no directory at all, and a directory
	// that does not exist. Neither is a reason to skip the check.
	noDir := mainPkg("internal/domain")
	noDir.Dir = ""

	missingDir := mainPkg("internal/domain/absent")

	// textRules are two rules over the same subtree, as .golangci.yml holds
	// adapter-helpers-do-no-io and jsondoc-parses-bytes: every rule whose
	// directory covers a package holds it, as depguard holds a file to every
	// rule whose globs match it, not only the rule with the longest directory.
	textRules := []check.Rule{
		{
			Name:  "outer",
			Dir:   "internal/domain",
			Allow: []string{"errors", "fmt", testModule + "/internal/domain/text"},
		},
		{
			Name:  "inner",
			Dir:   "internal/domain/text",
			Allow: []string{"strings"},
		},
	}

	textImportsErrors := mainPkg("internal/domain/text")
	textImportsErrors.Imports = []string{"errors"}

	textImportsStrings := mainPkg("internal/domain/text")
	textImportsStrings.Imports = []string{"strings"}

	textImportsBoth := mainPkg("internal/domain/text")
	textImportsBoth.Imports = []string{"errors", "strings"}

	textImportsNothing := mainPkg("internal/domain/text")

	domainXImportsStrings := mainPkg("internal/domain/x")
	domainXImportsStrings.Imports = []string{"strings"}

	tests := []struct {
		name string
		// rules are the directories of the module and their allow lists.
		rules []check.Rule
		// pkgs are the packages `go list -deps -json` reported.
		pkgs []check.Package
		// wantFlagged is the sorted import path of every package the check must
		// flag.
		wantFlagged []string
		// wantFragments are the values a message must name besides the import
		// path: the offending file, the import, and the rule that refuses it.
		wantFragments []string
		// notFragments are values no message may name.
		notFragments []string
	}{
		{
			name:  "a domain package importing only allowed packages passes",
			rules: domainRules,
			pkgs:  []check.Package{clean},
		},
		{
			name:  "a standard-library package is skipped",
			rules: domainRules,
			pkgs:  []check.Package{standardLibrary},
		},
		{
			name:        "a package of a nested module under the layer is flagged",
			rules:       domainRules,
			pkgs:        []check.Package{nestedModule},
			wantFlagged: []string{testModule + "/internal/domain"},
		},
		{
			name:        "a package without a module is flagged",
			rules:       domainRules,
			pkgs:        []check.Package{noModule},
			wantFlagged: []string{testModule + "/internal/domain"},
		},
		{
			name:        "a package outside every rule is flagged",
			rules:       domainRules,
			pkgs:        []check.Package{outside},
			wantFlagged: []string{testModule + "/internal/adapter/x"},
		},
		{
			name:          "the issue's assembly probe is flagged",
			rules:         domainRules,
			pkgs:          []check.Package{assembly},
			wantFlagged:   []string{testModule + "/internal/domain"},
			wantFragments: []string{"zz_arm64.s"},
		},
		{
			name:          "a cgo file is flagged",
			rules:         domainRules,
			pkgs:          []check.Package{cgoFiles},
			wantFlagged:   []string{testModule + "/internal/domain"},
			wantFragments: []string{"zz_cgo.go"},
		},
		{
			name:          "a C file is flagged",
			rules:         domainRules,
			pkgs:          []check.Package{cFiles},
			wantFlagged:   []string{testModule + "/internal/domain"},
			wantFragments: []string{"zz.c"},
		},
		{
			name:          "a syso file is flagged",
			rules:         domainRules,
			pkgs:          []check.Package{sysoFiles},
			wantFlagged:   []string{testModule + "/internal/domain"},
			wantFragments: []string{"zz.syso"},
		},
		{
			name:          "an import outside the allow list is flagged",
			rules:         domainRules,
			pkgs:          []check.Package{importsOS},
			wantFlagged:   []string{testModule + "/internal/domain"},
			wantFragments: []string{"os"},
		},
		{
			name:        "the issue's symlink probe is flagged",
			rules:       domainRules,
			pkgs:        []check.Package{symlinked},
			wantFlagged: []string{testModule + "/internal/domain/zzlink"},
		},
		{
			name:        "a package under a _ directory and one under testdata are flagged",
			rules:       domainRules,
			pkgs:        []check.Package{underscore, testdataDir},
			wantFlagged: []string{testModule + "/internal/domain/_x", testModule + "/internal/domain/testdata/y"},
		},
		{
			name:        "a package with no directory is flagged",
			rules:       domainRules,
			pkgs:        []check.Package{noDir},
			wantFlagged: []string{testModule + "/internal/domain"},
		},
		{
			name:        "a package whose directory does not exist is flagged",
			rules:       domainRules,
			pkgs:        []check.Package{missingDir},
			wantFlagged: []string{testModule + "/internal/domain/absent"},
		},
		{
			name:          "every covering rule applies: the inner rule flags an import it refuses",
			rules:         textRules,
			pkgs:          []check.Package{textImportsErrors},
			wantFlagged:   []string{testModule + "/internal/domain/text"},
			wantFragments: []string{"errors", "inner"},
		},
		{
			name:          "every covering rule applies: the outer rule flags an import it refuses",
			rules:         textRules,
			pkgs:          []check.Package{textImportsStrings},
			wantFlagged:   []string{testModule + "/internal/domain/text"},
			wantFragments: []string{"strings", "outer"},
		},
		{
			name:          "every covering rule applies: each refusing rule flags its own import",
			rules:         textRules,
			pkgs:          []check.Package{textImportsBoth},
			wantFlagged:   []string{testModule + "/internal/domain/text"},
			wantFragments: []string{"errors", "inner", "strings", "outer"},
		},
		{
			name:  "every covering rule applies: an import no covering rule refuses passes",
			rules: textRules,
			pkgs:  []check.Package{textImportsNothing},
		},
		{
			name:          "every covering rule applies: only the rule that covers the package flags it",
			rules:         textRules,
			pkgs:          []check.Package{domainXImportsStrings},
			wantFlagged:   []string{testModule + "/internal/domain/x"},
			wantFragments: []string{"strings", "outer"},
			notFragments:  []string{"inner"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			messages := check.Packages(root, testModule, tc.rules, tc.pkgs)

			got := flaggedPaths(t, messages, tc.pkgs)
			if diff := cmp.Diff(tc.wantFlagged, got); diff != "" {
				t.Errorf("flagged packages (-want +got):\n%s\nmessages:\n%s", diff, strings.Join(messages, "\n"))
			}

			for _, fragment := range tc.wantFragments {
				var found []string
				for _, message := range messages {
					if strings.Contains(message, fragment) {
						found = append(found, message)
					}
				}

				if len(found) != 1 {
					t.Errorf("%d messages name %q, want exactly 1\nmessages:\n%s",
						len(found), fragment, strings.Join(messages, "\n"))
				}
			}

			for _, fragment := range tc.notFragments {
				for _, message := range messages {
					if strings.Contains(message, fragment) {
						t.Errorf("message %q names %q, which no rule of this case holds the package to\nmessages:\n%s",
							message, fragment, strings.Join(messages, "\n"))
					}
				}
			}
		})
	}
}

// TestSources pins the pass over every package of the module, layer or
// not: each must belong to the main module, sit in the directory its import
// path names with symlinks resolved, and hold no source but Go files. Imports
// are not this check's business, and a package outside the module path is
// skipped: on a release build, the target adapters embed their snapshots and
// their assembly would be invisible to every import rule, wherever it sat.
func TestSources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	for _, dir := range []string{
		"internal/adapter/target/x",
		"internal/adapter/target/y",
		"internal/adapter/target/outside",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o700); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}

	// A package directory that is a symlink to a directory elsewhere: lint
	// reads its files under another path.
	if err := os.Symlink(
		filepath.Join(root, "internal", "adapter", "target", "outside"),
		filepath.Join(root, "internal", "adapter", "target", "zzlink"),
	); err != nil {
		t.Fatalf("create the symlink probe: %v", err)
	}

	mainModule := &check.Module{Path: testModule, Main: true}

	// mainPkg is a package of the main module in dir, with no source files: a
	// case sets the one field it breaks on the copy it gets.
	mainPkg := func(dir string) check.Package {
		return check.Package{
			ImportPath: testModule + "/" + dir,
			Dir:        filepath.Join(root, filepath.FromSlash(dir)),
			Module:     mainModule,
		}
	}

	clean := mainPkg("internal/adapter/target/x")

	// The auditor's scenario: assembly in an embedded-snapshot target.
	assembly := mainPkg("internal/adapter/target/y")
	assembly.SFiles = []string{"zz_amd64.s"}

	symlinked := mainPkg("internal/adapter/target/zzlink")

	nestedModule := mainPkg("internal/adapter/target/x")
	nestedModule.Module = &check.Module{Path: testModule + "/internal/adapter/target"}

	// A third-party package with assembly of its own, and a standard-library
	// package: both are outside the module path and skipped, whatever they
	// hold.
	thirdParty := check.Package{
		ImportPath: "example.org/sdk",
		Dir:        filepath.Join(root, "sdk"),
		Module:     &check.Module{Path: "example.org/sdk"},
		SFiles:     []string{"zz_amd64.s"},
	}

	standardLibrary := check.Package{
		ImportPath: "runtime",
		Standard:   true,
		SFiles:     []string{"zz.s"},
	}

	noDir := mainPkg("internal/adapter/target/x")
	noDir.Dir = ""

	missingDir := mainPkg("internal/adapter/target/absent")

	tests := []struct {
		name          string
		pkgs          []check.Package
		wantFlagged   []string
		wantFragments []string
	}{
		{
			name: "a main-module package outside every layer passes",
			pkgs: []check.Package{clean},
		},
		{
			name:          "assembly in a target package is flagged",
			pkgs:          []check.Package{assembly},
			wantFlagged:   []string{testModule + "/internal/adapter/target/y"},
			wantFragments: []string{"zz_amd64.s"},
		},
		{
			name:        "a symlinked package directory is flagged",
			pkgs:        []check.Package{symlinked},
			wantFlagged: []string{testModule + "/internal/adapter/target/zzlink"},
		},
		{
			name:        "a package of a nested module under the module is flagged",
			pkgs:        []check.Package{nestedModule},
			wantFlagged: []string{testModule + "/internal/adapter/target/x"},
		},
		{
			name: "a third-party package with assembly is skipped",
			pkgs: []check.Package{thirdParty},
		},
		{
			name: "a standard-library package is skipped",
			pkgs: []check.Package{standardLibrary},
		},
		{
			name:        "a package with no directory is flagged",
			pkgs:        []check.Package{noDir},
			wantFlagged: []string{testModule + "/internal/adapter/target/x"},
		},
		{
			name:        "a package whose directory does not exist is flagged",
			pkgs:        []check.Package{missingDir},
			wantFlagged: []string{testModule + "/internal/adapter/target/absent"},
		},
		{
			name: "the binary's own packages and the third-party ones together are read one by one",
			pkgs: []check.Package{clean, assembly, thirdParty, standardLibrary},
			// Only the module's own package with assembly is flagged.
			wantFlagged:   []string{testModule + "/internal/adapter/target/y"},
			wantFragments: []string{"zz_amd64.s"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			messages := check.Sources(root, testModule, tc.pkgs)

			got := flaggedPaths(t, messages, tc.pkgs)
			if diff := cmp.Diff(tc.wantFlagged, got); diff != "" {
				t.Errorf("flagged packages (-want +got):\n%s\nmessages:\n%s", diff, strings.Join(messages, "\n"))
			}

			for _, fragment := range tc.wantFragments {
				var found []string

				for _, message := range messages {
					if strings.Contains(message, fragment) {
						found = append(found, message)
					}
				}

				if len(found) != 1 {
					t.Errorf("%d messages name %q, want exactly 1\nmessages:\n%s",
						len(found), fragment, strings.Join(messages, "\n"))
				}
			}
		})
	}
}

// flaggedPaths returns the import path of the package each message names, once
// per package and sorted, and fails the test for a message that names no
// package of the case or more than one: every message must name the import path
// of the package it flags.
func flaggedPaths(t *testing.T, messages []string, pkgs []check.Package) []string {
	t.Helper()

	var flagged []string
	seen := make(map[string]bool, len(messages))

	for _, message := range messages {
		var named []string

		for _, pkg := range pkgs {
			if strings.Contains(message, pkg.ImportPath) {
				named = append(named, pkg.ImportPath)
			}
		}

		switch len(named) {
		case 1:
			if !seen[named[0]] {
				seen[named[0]] = true
				flagged = append(flagged, named[0])
			}
		case 0:
			t.Errorf("message %q names no package of this case", message)
		default:
			t.Errorf("message %q names more than one package: %s", message, strings.Join(named, ", "))
		}
	}

	slices.Sort(flagged)

	return flagged
}
