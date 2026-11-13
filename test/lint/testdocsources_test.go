package lint_test

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// docRule is the strict depguard rule .golangci.yml holds the test trees' own
// documentation packages under (issue #152): test/apiparity, test/architecture,
// test/container, test/devenv, test/docs, test/e2e, test/examples, test/lint and
// test/openapibundle, each a package a composition root may import. Its allow
// list is empty, so no import of theirs lints. That closes os/exec, unsafe,
// os/user and every other package there, but it has nothing to say about a file
// that needs no import at all: an assembly file beside a bodyless Go func can
// issue a raw execve or read envp, and neither depguard nor forbidigo sees it
// (issue #194, the audit of #185).
//
// The checks below read the trees of that rule from its files: list, so a tree
// the rule takes on is held by them without a second edit.
const docRule = "test-docs-are-pure"

// nonGoSourceExtensions are the extensions of the files the Go toolchain builds
// beside the Go source of a package, which package source is but Go source is
// not: assembly (.s, .S), SWIG (.sx, .swig, .swigcxx), C (.c, .h), C++ (.cc,
// .cpp, .cxx, .hh, .hpp, .hxx), Objective-C (.m), Fortran (.f, .F, .for, .f90)
// and the compiled system object (.syso). The case of .s beside .S, and of .f
// beside .F, is the toolchain's: the capital letter marks a file its
// preprocessor reads, so both are sources of a package.
var nonGoSourceExtensions = []string{
	".s", ".S", ".sx",
	".c", ".h", ".cc", ".cpp", ".cxx", ".hh", ".hpp", ".hxx",
	".m",
	".f", ".F", ".for", ".f90",
	".syso",
	".swig", ".swigcxx",
}

// docSourceLine returns the problem docSourceProblems reports for rel, a file at
// a non-Go source extension under dir, one of the trees the rule
// test-docs-are-pure holds. It is the one wording the check and the cases below
// share, so a fixture states which files are refused rather than how the check
// words a refusal.
func docSourceLine(rel, dir string) string {
	return fmt.Sprintf("%s: a %s file is a Go-toolchain source that needs no import, so neither "+
		"depguard's %s nor a forbidigo group reads the code it carries; %s is held to Go source "+
		"only, so the file belongs outside the tree (issue #194)",
		rel, path.Ext(rel), docRule, dir)
}

// TestDocPackagesHoldNoNonGoSource checks every file of every tree the depguard
// rule test-docs-are-pure holds for a non-Go source (docSourceProblems): the
// repository holds none today, and the test pins that. An assembly file under
// test/e2e, or under test/lint itself, would carry code into a package a
// composition root may import, and neither depguard's allow list of nothing nor
// a forbidigo group would read it.
//
// It walks the module root with skipDir, the walk of scanSources, so a testdata
// directory, a directory whose name starts with . or _ and the root of another
// module are left out exactly as the go command leaves them out: testdata is no
// package source, so a .s file there is a fixture, as in
// testdata/docsources/mod below.
func TestDocPackagesHoldNoNonGoSource(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	held, takenOut := docTreesOf(t, root)

	files := filesUnder(t, root, skipDir)
	if len(files) == 0 {
		t.Fatalf("no file under %s", root)
	}

	for _, problem := range docSourceProblems(files, held, takenOut) {
		t.Error(problem)
	}

	// A tree the rule holds that the walk never reached would pass for the
	// wrong reason: the check would hold nothing there.
	for _, dir := range held {
		if !slices.ContainsFunc(files, func(rel string) bool {
			return strings.HasPrefix(rel, dir+"/")
		}) {
			t.Errorf("the walk of %s found no file under %s, a tree %s holds", root, dir, docRule)
		}
	}
}

// TestDocPackageNonGoSourceProbe runs the check over the probe tree
// testdata/docsources/mod, laid out as the repository's test directory is: an
// assembly file under test/e2e must be refused, and the one under
// test/e2e/testdata, which is no package source, must not be. The repository
// holds no such file, so without the probe nothing would prove the check bites
// (the pin of issue #194).
func TestDocPackageNonGoSourceProbe(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	held, takenOut := docTreesOf(t, root)

	const dir = "test/lint/testdata/docsources/mod"
	files := filesUnder(t, filepath.Join(root, filepath.FromSlash(dir)), nil)

	// The probe holds both fixtures: one the check refuses, one it must let
	// through, so a walk that read nothing would fail here rather than pass.
	for _, want := range []string{"test/e2e/probe_amd64.s", "test/e2e/testdata/probe_amd64.s"} {
		if !slices.Contains(files, want) {
			t.Fatalf("the probe %s holds no %s", dir, want)
		}
	}

	want := []string{docSourceLine("test/e2e/probe_amd64.s", "test/e2e")}
	if diff := cmp.Diff(want, docSourceProblems(files, held, takenOut), cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("docSourceProblems() on the probe mismatch (-want +got):\n%s", diff)
	}
}

// docSourceCase is one case of TestDocSourceProblems: the files of a walked
// directory, the trees the rule holds and the trees it takes out, and the
// problems docSourceProblems must report for them. A case that sets neither
// tree is read against docSourceHeld and docSourceTakenOut.
type docSourceCase struct {
	name     string
	files    []string
	held     []string
	takenOut []string
	want     []string
}

// docSourceHeld are the trees the cases of TestDocSourceProblems hold by
// default, two of the rule's directories: test/e2e, which holds test files and
// doc.go, and test/lint.
var docSourceHeld = []string{"test/e2e", "test/lint"}

// docSourceTakenOut are the trees the cases take out by default, as the rule
// takes test/architecture/internal out: the package check there reads os,
// os/exec and test/internal/releasetarget.
var docSourceTakenOut = []string{"test/architecture/internal"}

// TestDocSourceProblems pins docSourceProblems, the seam of
// TestDocPackagesHoldNoNonGoSource and TestDocPackageNonGoSourceProbe, on the
// paths a walk hands it: every extension of the toolchain's non-Go sources, the
// files that are no source at all and pass, the testdata directory that is no
// package source, the tree the rule takes out, and the trees it does not hold,
// whose files are no concern of this rule. Each refusal is one line, in the
// order the walk handed the files over.
func TestDocSourceProblems(t *testing.T) {
	t.Parallel()

	cases := []docSourceCase{
		{
			name:  "an assembly file beside a bodyless Go func",
			files: []string{"test/e2e/probe_amd64.s"},
			want:  []string{docSourceLine("test/e2e/probe_amd64.s", "test/e2e")},
		},
		{
			name:  "a C source",
			files: []string{"test/e2e/probe.c"},
			want:  []string{docSourceLine("test/e2e/probe.c", "test/e2e")},
		},
		{
			name:  "a compiled system object",
			files: []string{"test/lint/syso_amd64.syso"},
			want:  []string{docSourceLine("test/lint/syso_amd64.syso", "test/lint")},
		},
		{
			// The pin of the whole rule: the file a package's Go source sits
			// beside is no concern of an import list.
			name:  "the files under one tree are each refused",
			files: []string{"test/e2e/a.c", "test/e2e/a.go", "test/e2e/a.s"},
			want:  []string{docSourceLine("test/e2e/a.c", "test/e2e"), docSourceLine("test/e2e/a.s", "test/e2e")},
		},
		{
			name: "the files a doc-only tree may hold",
			files: []string{
				"test/e2e/doc.go",
				"test/e2e/runs_test.go",
				"test/e2e/testdata/inventory.json",
				"test/e2e/README.md",
				"test/lint/probe.sh",
				"test/lint/assess.golden",
				"test/lint/.golangci.yml",
			},
		},
		{
			// testdata is no package source, so a non-Go source there is a
			// fixture: the e2e tier keeps its inventories and goldens there.
			name: "a file under a testdata directory is exempt",
			files: []string{
				"test/e2e/testdata/probe_amd64.s",
				"test/e2e/runs/testdata/deep/probe.c",
			},
		},
		{
			name:  "a directory that only starts with testdata is no testdata directory",
			files: []string{"test/e2e/mytestdata/probe_amd64.s"},
			want:  []string{docSourceLine("test/e2e/mytestdata/probe_amd64.s", "test/e2e")},
		},
		{
			name:     "a tree the rule takes out is no tree",
			held:     []string{"test/architecture"},
			takenOut: []string{"test/architecture/internal"},
			files:    []string{"test/architecture/internal/check/probe_amd64.s"},
		},
		{
			name:     "a directory that only starts with a tree the rule takes out is held by it",
			held:     []string{"test/architecture"},
			takenOut: []string{"test/architecture/internal"},
			files:    []string{"test/architecture/internalcheck/probe_amd64.s"},
			want: []string{docSourceLine("test/architecture/internalcheck/probe_amd64.s",
				"test/architecture")},
		},
		{
			name:  "a tree the rule does not hold is no concern of this rule",
			files: []string{"test/architecture/probe_amd64.s", "internal/cli/probe.c", "testdata/probe.s"},
		},
		{
			name:  "a directory that only starts with a held tree is no tree of it",
			files: []string{"test/e2e2/probe_amd64.s", "test/e2ex/y/probe.s"},
		},
		{
			name:     "no tree held holds nothing",
			files:    []string{"test/e2e/probe_amd64.s"},
			held:     []string{},
			takenOut: []string{},
		},
	}
	for _, ext := range nonGoSourceExtensions {
		rel := "test/e2e/probe_amd64" + ext
		cases = append(cases, docSourceCase{
			name:  "a " + ext + " file, which the toolchain builds beside the Go source",
			files: []string{rel},
			want:  []string{docSourceLine(rel, "test/e2e")},
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			held, takenOut := tc.held, tc.takenOut
			if held == nil {
				held = docSourceHeld
			}
			if takenOut == nil {
				takenOut = docSourceTakenOut
			}

			got := docSourceProblems(tc.files, held, takenOut)
			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("docSourceProblems() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestDocTrees pins docTrees, which reads the trees of the rule from
// .golangci.yml rather than a list kept here: the rule's files: list is what
// holds those packages, so the check must hold the same trees, and a tree the
// rule takes on is covered without a second edit. It reads only the shapes the
// rule writes today, a held directory as **/<dir>/** and one taken out as
// !**/<dir>/**, and !$test, which only takes the _test.go files out and names
// no directory; anything else is an error, so a list written another way is
// never silently read as no tree at all.
func TestDocTrees(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		doc      string
		held     []string
		takenOut []string
		wantErr  bool
	}{
		{
			name: "the rule as .golangci.yml writes it",
			doc:  docRuleDoc(`"**/test/e2e/**"`, `"!$test"`),
			held: []string{"test/e2e"},
		},
		{
			name:     "a tree the rule takes out",
			doc:      docRuleDoc(`"**/test/architecture/**"`, `"!**/test/architecture/internal/**"`, `"!$test"`),
			held:     []string{"test/architecture"},
			takenOut: []string{"test/architecture/internal"},
		},
		{
			name:     "the order the rule writes",
			doc:      docRuleDoc(`"**/test/lint/**"`, `"**/test/e2e/**"`, `"!**/test/lint/testdata/**"`),
			held:     []string{"test/lint", "test/e2e"},
			takenOut: []string{"test/lint/testdata"},
		},
		{
			name:    "a rule .golangci.yml does not hold",
			doc:     "linters:\n  settings:\n    depguard:\n      rules:\n        domain-is-pure:\n          list-mode: strict\n          files:\n            - \"**/internal/domain/**\"\n",
			wantErr: true,
		},
		{
			name:    "a files list that names no directory",
			doc:     docRuleDoc(`"**/test/e2e/**"`, `"$all"`),
			wantErr: true,
		},
		{
			name:    "a files list that names a file glob",
			doc:     docRuleDoc(`"**/test/e2e/**/*_test.go"`),
			wantErr: true,
		},
		{
			name:    "a rule with no files list",
			doc:     "linters:\n  settings:\n    depguard:\n      rules:\n        test-docs-are-pure:\n          list-mode: strict\n          allow: []\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			held, takenOut, err := docTrees([]byte(tt.doc))
			if (err != nil) != tt.wantErr {
				t.Fatalf("docTrees() error = %v, want an error: %t", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if diff := cmp.Diff(tt.held, held, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("docTrees() held mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.takenOut, takenOut, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("docTrees() takenOut mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// docRuleDoc returns a .golangci.yml whose depguard rules hold the one rule
// test-docs-are-pure, its files: list the entries files, each written as the
// rule writes it (quotes included). It is the shape .golangci.yml has on the
// path docTrees reads.
func docRuleDoc(files ...string) string {
	var b strings.Builder
	b.WriteString("linters:\n  settings:\n    depguard:\n      rules:\n        test-docs-are-pure:\n" +
		"          list-mode: strict\n          files:\n")
	for _, f := range files {
		fmt.Fprintf(&b, "            - %s\n", f)
	}
	b.WriteString("          allow: []\n          deny:\n            - pkg: os\n" +
		"              desc: a doc-only package imports nothing\n")

	return b.String()
}

// docTreesOf returns the trees of the depguard rule test-docs-are-pure, as the
// .golangci.yml at root writes them (docTrees): the directories the checks of
// this file hold, and the ones they take out.
func docTreesOf(t *testing.T, root string) (held, takenOut []string) {
	t.Helper()

	doc, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}

	held, takenOut, err = docTrees(doc)
	if err != nil {
		t.Fatalf("read depguard rule %s: %v", docRule, err)
	}
	if len(held) == 0 {
		t.Fatalf(".golangci.yml holds no tree for depguard rule %s", docRule)
	}

	return held, takenOut
}

// filesUnder returns the slash-separated paths, relative to dir, of every file
// under it, in the lexical order of fs.WalkDir, leaving out the directories
// skip reports (none when skip is nil) and reading no file. walkGoFiles reads
// the Go files of a tree; this is the walk of the paths themselves, which is
// what a check of the files beside them needs (issue #194).
func filesUnder(t *testing.T, dir string, skip func(r *os.Root, rel, name string) (bool, error)) []string {
	t.Helper()

	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	var files []string
	err = fs.WalkDir(r.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == "." || skip == nil {
				return nil
			}
			skipped, err := skip(r, rel, d.Name())
			if err != nil || !skipped {
				return err
			}

			return fs.SkipDir
		}
		files = append(files, rel)

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}

	return files
}
