package lint_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// compositionRootDirs are the two composition roots (issue #111): cmd/shift2eu
// and cmd/shift2eu-server. Every Go file under them, _test.go files included, is
// held to the rules below, which no golangci-lint linter can state.
var compositionRootDirs = []string{"cmd/shift2eu", "cmd/shift2eu-server"}

// TestCompositionRootPassesLookupEnvToRun checks cmd/shift2eu-server/main.go, the
// one composition root file that reads an environment variable. Forbidigo allows
// os.LookupEnv in it for the SHIFT2EU_SERVER_TOKEN lookup it hands to run (ADR
// 0019), and the exception is a rule about a path and a msg, not about the
// argument: any second use of os.LookupEnv in the file lints clean. A test reads
// the file on the AST, so the exception is the argument run takes, in that one
// reference, and nothing else.
func TestCompositionRootPassesLookupEnvToRun(t *testing.T) {
	t.Parallel()

	const rel = "cmd/shift2eu-server/main.go"
	src, err := os.ReadFile(filepath.Join(moduleRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	problems, err := lookupEnvProblems(rel, src)
	if err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// lookupEnvProblems returns what is wrong with the use of os.LookupEnv in src,
// the content of the composition root file at rel, one line each, and nothing
// when the file is as ADR 0019 has it: exactly one reference to os.LookupEnv,
// passed directly to run, which reads SHIFT2EU_SERVER_TOKEN with it. A second
// reference, a call of os.LookupEnv itself, an assignment, and a reference passed
// to anything but run are each refused, and so is a file that dot-imports os,
// which hides the argument behind a bare name.
//
// It reads the source, not the linted tree: a case is a string, so the pin is a
// fixture (TestLookupEnvProblems).
func lookupEnvProblems(rel string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	name, at, ok := importedName(f, "os")
	if !ok {
		return []string{fmt.Sprintf("%s: imports no package os, so nothing passes os.LookupEnv to run", rel)}, nil
	}
	if name == "." {
		return []string{fmt.Sprintf("%s:%d: the file dot-imports os, which hides whether os.LookupEnv is the argument run takes; import it as os", rel, fset.Position(at).Line)}, nil
	}

	var refs []token.Pos
	passed := map[token.Pos]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if isName(n.X, name) && n.Sel.Name == "LookupEnv" {
				refs = append(refs, n.Pos())
			}
		case *ast.CallExpr:
			if !isName(n.Fun, "run") {
				break
			}
			for _, arg := range n.Args {
				if sel, ok := arg.(*ast.SelectorExpr); ok && isName(sel.X, name) && sel.Sel.Name == "LookupEnv" {
					passed[sel.Pos()] = true
				}
			}
		}
		return true
	})

	switch {
	case len(refs) == 0:
		return []string{fmt.Sprintf("%s: no reference to os.LookupEnv; main passes it to run, which reads SHIFT2EU_SERVER_TOKEN with it (ADR 0019)", rel)}, nil
	case len(refs) > 1:
		problems := make([]string, 0, len(refs))
		for i, pos := range refs {
			problems = append(problems, fmt.Sprintf("%s:%d: os.LookupEnv is reference %d of %d in this file; a composition root holds exactly one, passed to run", rel, fset.Position(pos).Line, i+1, len(refs)))
		}
		return problems, nil
	case !passed[refs[0]]:
		return []string{fmt.Sprintf("%s:%d: os.LookupEnv is not a direct argument of run; the composition root's one lookup is the argument run takes, never a call of its own, an assignment or another function's argument", rel, fset.Position(refs[0]).Line)}, nil
	}
	return nil, nil
}

// importedName returns the name f gives the package at path, the position of its
// import and whether f imports it. A dot import is returned as ".".
func importedName(f *ast.File, path string) (string, token.Pos, bool) {
	for _, imp := range f.Imports {
		if imp.Path.Value != strconv.Quote(path) {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name, imp.Pos(), true
		}
		return path, imp.Pos(), true
	}
	return "", token.NoPos, false
}

// isName reports whether expr is the identifier name, as os.LookupEnv writes the
// package os and run(os.LookupEnv) writes the function run.
func isName(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}

// TestLookupEnvProblems pins lookupEnvProblems, the seam of
// TestCompositionRootPassesLookupEnvToRun: a fixture with a second use of
// os.LookupEnv fails, as the rule that would allow it does.
func TestLookupEnvProblems(t *testing.T) {
	t.Parallel()

	const rel = "cmd/shift2eu-server/main.go"
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "the one lookup, passed to run",
			src: `package main

import "os"

func main() { run(os.LookupEnv) }

func run(lookup func(string) (string, bool)) {}
`,
		},
		{
			name: "an aliased import is the same package",
			src: `package main

import stdos "os"

func main() { run(stdos.LookupEnv) }

func run(lookup func(string) (string, bool)) {}
`,
		},
		{
			// The pin: the exception is one argument, and a secret read
			// beside it lints clean today.
			name: "a second use is refused",
			src: `package main

import "os"

func main() {
	run(os.LookupEnv)
	_, _ = os.LookupEnv("AWS_SECRET_ACCESS_KEY")
}

func run(lookup func(string) (string, bool)) {}
`,
			want: []string{
				rel + ":6: os.LookupEnv is reference 1 of 2 in this file; a composition root holds exactly one, passed to run",
				rel + ":7: os.LookupEnv is reference 2 of 2 in this file; a composition root holds exactly one, passed to run",
			},
		},
		{
			name: "a lookup called in the file is refused",
			src: `package main

import "os"

func main() { _ = os.LookupEnv("SHIFT2EU_SERVER_TOKEN") }
`,
			want: []string{
				rel + ":5: os.LookupEnv is not a direct argument of run; the composition root's one lookup is the argument run takes, never a call of its own, an assignment or another function's argument",
			},
		},
		{
			name: "a lookup assigned to a variable is refused",
			src: `package main

import "os"

func main() {
	lookup := os.LookupEnv
	run(lookup)
}

func run(lookup func(string) (string, bool)) {}
`,
			want: []string{
				rel + ":6: os.LookupEnv is not a direct argument of run; the composition root's one lookup is the argument run takes, never a call of its own, an assignment or another function's argument",
			},
		},
		{
			name: "a lookup another function wraps is refused",
			src: `package main

import "os"

func main() { run(decorate(os.LookupEnv)) }

func decorate(lookup func(string) (string, bool)) func(string) (string, bool) { return lookup }

func run(lookup func(string) (string, bool)) {}
`,
			want: []string{
				rel + ":5: os.LookupEnv is not a direct argument of run; the composition root's one lookup is the argument run takes, never a call of its own, an assignment or another function's argument",
			},
		},
		{
			name: "a file that reads nothing is refused",
			src: `package main

import "os"

func main() { os.Exit(0) }
`,
			want: []string{
				rel + ": no reference to os.LookupEnv; main passes it to run, which reads SHIFT2EU_SERVER_TOKEN with it (ADR 0019)",
			},
		},
		{
			name: "a dot import of os is refused",
			src: `package main

import . "os"

func main() { run(LookupEnv) }

func run(lookup func(string) (string, bool)) {}
`,
			want: []string{
				rel + ":3: the file dot-imports os, which hides whether os.LookupEnv is the argument run takes; import it as os",
			},
		},
		{
			name: "a file that imports no os is refused",
			src: `package main

func main() { run(nil) }

func run(lookup func(string) (string, bool)) {}
`,
			want: []string{
				rel + ": imports no package os, so nothing passes os.LookupEnv to run",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := lookupEnvProblems(rel, []byte(tt.src))
			if err != nil {
				t.Fatalf("lookupEnvProblems(): %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("lookupEnvProblems() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// environmentPathLiterals are the paths that hold every environment variable of
// this process: the environ file of its own /proc entry, the descriptor
// directory, whose numbers include the one environ is, and the per-process
// entries of every other process. Reading one dumps the environment, credentials
// included (CLAUDE.md rule 3), and no forbidigo group can say so: the call is
// os.ReadFile, or io.ReadAll over an os.Open, and its argument is a string.
var environmentPathLiterals = []string{"/proc/", "/dev/fd/"}

// environmentPathProblems returns what is wrong with the string literals of src,
// the content of the Go file at rel, one line each: a literal that holds one of
// environmentPathLiterals names the environment of a process, which a
// composition root never reads, its tests included. It reads a literal as the
// compiler does, so a raw and a quoted string are read alike; a path assembled
// from parts at run time, such as filepath.Join("/proc", "self", "environ"), is
// beyond a check of literals, and depguard and TestSourcesHideNothingFromDepguard
// are what keep os out of the layers below.
func environmentPathProblems(rel string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var problems []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true // a literal the parser read and the compiler refuses
		}
		for _, path := range environmentPathLiterals {
			if strings.Contains(value, path) {
				problems = append(problems, fmt.Sprintf("%s:%d: the string literal %s holds %q, which names every environment variable of this process, credentials included; a composition root reads the named variable it needs and nothing else", rel, fset.Position(lit.Pos()).Line, lit.Value, path))
				break
			}
		}
		return true
	})
	return problems, nil
}

// TestCompositionRootsNameNoEnvironmentPath checks every Go file of the two
// composition roots, _test.go files included, for a literal that names the
// environment of a process (environmentPathProblems). The tree holds none today,
// and the test pins that: os.ReadFile("/proc/self/environ") in a composition root
// would hand out every variable of the server, the token included, and no rule of
// the two groups catches a read of a path.
func TestCompositionRootsNameNoEnvironmentPath(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	files := 0
	refused, err := walkGoFiles(root, skipDir, func(rel string, src []byte) error {
		if !slices.ContainsFunc(compositionRootDirs, func(dir string) bool {
			return strings.HasPrefix(rel, dir+"/")
		}) {
			return nil
		}
		files++
		problems, err := environmentPathProblems(rel, src)
		if err != nil {
			return err
		}
		for _, p := range problems {
			t.Error(p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	for _, p := range refused {
		t.Error(p)
	}
	if files == 0 {
		t.Fatalf("no Go file under %v", compositionRootDirs)
	}
}

// TestEnvironmentPathProblems pins environmentPathProblems, the seam of
// TestCompositionRootsNameNoEnvironmentPath, on fixtures: the tree holds no such
// literal, so a fixture is what proves the check would refuse one.
func TestEnvironmentPathProblems(t *testing.T) {
	t.Parallel()

	const rel = "cmd/shift2eu-server/read.go"
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "a path of the root's own",
			src:  "package main\n\nvar config = \"/etc/shift2eu/config.json\"\n",
		},
		{
			name: "the environment of this process",
			src:  "package main\n\nimport \"os\"\n\nfunc read() { _, _ = os.ReadFile(\"/proc/self/environ\") }\n",
			want: []string{
				rel + ":5: the string literal \"/proc/self/environ\" holds \"/proc/\", which names every environment variable of this process, credentials included; a composition root reads the named variable it needs and nothing else",
			},
		},
		{
			name: "the environment of this process, as a raw string",
			src:  "package main\n\nimport \"os\"\n\nfunc read() { _, _ = os.ReadFile(`/proc/self/environ`) }\n",
			want: []string{
				rel + ":5: the string literal `/proc/self/environ` holds \"/proc/\", which names every environment variable of this process, credentials included; a composition root reads the named variable it needs and nothing else",
			},
		},
		{
			name: "a descriptor of this process",
			src:  "package main\n\nimport \"os\"\n\nfunc read() { _, _ = os.ReadFile(\"/dev/fd/3\") }\n",
			want: []string{
				rel + ":5: the string literal \"/dev/fd/3\" holds \"/dev/fd/\", which names every environment variable of this process, credentials included; a composition root reads the named variable it needs and nothing else",
			},
		},
		{
			name: "a test file is held to it too",
			src:  "package main\n\nimport \"os\"\n\nvar _ = os.ReadFile(\"/proc/1/environ\")\n",
			want: []string{
				rel + ":5: the string literal \"/proc/1/environ\" holds \"/proc/\", which names every environment variable of this process, credentials included; a composition root reads the named variable it needs and nothing else",
			},
		},
		{
			name: "a comment is no read",
			src:  "package main\n\n// os.ReadFile(\"/proc/self/environ\") would hand out the token.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := environmentPathProblems(rel, []byte(tt.src))
			if err != nil {
				t.Fatalf("environmentPathProblems(): %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("environmentPathProblems() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
