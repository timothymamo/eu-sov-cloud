package lint_test

import (
	"path"
	"slices"
	"strings"
	"testing"
)

// TestLintLoadsEveryGoFile fails on every Go file of the module that
// golangci-lint never loads, so that no lint rule checks it, the
// cloud-read-only and dependency rules included. make lint and every entry of
// CI's lint-target matrix lint the host and every release target of
// .goreleaser.yaml, with the tags of run.build-tags (lintTags). This test
// lists only what builds for linux on amd64, one of those targets: a file
// behind another tag, //go:build ignore among them, can still run with real
// credentials, and a file only for another platform, though linted, fails here
// until this test lists that platform too. It asks the go command which files
// it loads and reads the module's files with walkGoFiles, the walk of
// TestSourcesHideNothingFromDepguard. The files in directories ./... skips are
// left to TestLintVisitsEveryImportedPackage.
func TestLintLoadsEveryGoFile(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	loaded := map[string]bool{}
	for _, p := range goList(t, root, "Dir,GoFiles,CgoFiles,TestGoFiles,XTestGoFiles,InvalidGoFiles", "./...") {
		if p.Dir == "" {
			continue
		}
		dir, ok := relToRoot(t, root, p.Dir)
		if !ok {
			continue
		}
		// An invalid file is loaded too: lint reports the error.
		for _, files := range [][]string{p.GoFiles, p.CgoFiles, p.TestGoFiles, p.XTestGoFiles, p.InvalidGoFiles} {
			for _, f := range files {
				loaded[path.Join(dir, f)] = true
			}
		}
	}

	for _, f := range moduleGoFiles(t, root) {
		if loaded[f.rel] {
			continue
		}
		base := path.Base(f.rel)
		if strings.HasPrefix(base, "_") || strings.HasPrefix(base, ".") {
			t.Errorf("%s: the go command ignores a file whose name starts with _ or ., so golangci-lint never "+
				"lints it; rename or delete it", f.rel)
			continue
		}
		t.Errorf("%s: golangci-lint never loads this file, so no lint rule checks it: its build constraints or "+
			"its name leave it out of GOOS=linux GOARCH=amd64 with the tags %s. Use no build tag but "+
			"integration or e2e (never //go:build ignore). make lint and CI already lint every release target of "+
			".goreleaser.yaml, so for a file only for another such platform, make this test list that "+
			"platform too", f.rel, lintTags)
	}
}

// TestLintVisitsEveryImportedPackage fails on every package of the module's
// directory tree that the module's code imports, tests included, but that
// golangci-lint never lints: ./... skips testdata directories, directories
// whose name starts with _ or ., and the root of another module, while an
// import of a package there still builds and runs.
func TestLintVisitsEveryImportedPackage(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	var problems []string
	seen := map[string]bool{}
	for _, p := range goList(t, root, "ImportPath,Dir,Module", "-deps", "-test", "./...") {
		if p.Dir == "" {
			continue
		}
		dir, ok := relToRoot(t, root, p.Dir)
		if !ok || seen[dir] {
			continue
		}
		seen[dir] = true

		if skipped := slices.ContainsFunc(strings.Split(dir, "/"), func(elem string) bool {
			return elem == "testdata" || strings.HasPrefix(elem, "_") || (strings.HasPrefix(elem, ".") && elem != ".")
		}); skipped {
			problems = append(problems, dir+": a package here is imported, but ./... skips testdata, _ and . "+
				"directories, so golangci-lint never lints it and no lint rule checks it; move the package "+
				"out of that directory")
			continue
		}
		if p.Module == nil || !p.Module.Main {
			problems = append(problems, dir+": a package here is imported from a module nested in this one, "+
				"which golangci-lint does not lint from here; move the package into this module")
		}
	}
	slices.Sort(problems)
	for _, problem := range problems {
		t.Error(problem)
	}
}
