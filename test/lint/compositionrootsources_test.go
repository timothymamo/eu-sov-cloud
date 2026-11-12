package lint_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
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
// the file on the AST, so the exception is the token source run takes, in that
// one reference, and nothing else: run takes a func() (string, bool) with no
// name, and the name its one os.LookupEnv call reads is tokenEnvVar's.
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

// envVarName is the name of the environment variable the server reads its
// bearer token from (cmd/shift2eu-server/token.go): the one name the composition
// root's token source reads, and no other name or literal. It is a variable's
// name, not a value, so gosec's G101 has nothing to report here.
const envVarName = "SHIFT2EU_SERVER_TOKEN"

// envVarIdent is the identifier of the constant that holds envVarName. run
// takes a token source with no name parameter, so the name its one os.LookupEnv
// call reads is the literal's, and the argument is this identifier alone: a
// second name, in this file or in any other, is no code path a composition root
// opens.
const envVarIdent = "tokenEnvVar"

// lookupEnvProblems returns what is wrong with the use of os.LookupEnv in src,
// the content of the composition root file at rel, one line each, and nothing
// when the file is as ADR 0019 has it: exactly one reference to os.LookupEnv, the
// call that is the sole return expression of the func() (string, bool) main passes
// to run as an argument, and its one argument is the identifier tokenEnvVar. A
// second reference, the old form that hands os.LookupEnv itself to run, a call of
// os.LookupEnv elsewhere, another variable's name or a string literal, and a
// func literal that is not the sole return of the source are each refused, and so
// are a file that dot-imports os, which hides the argument behind a bare name,
// a file that declares or assigns run, so that the call main makes is no call of
// the package's run, a file that declares or assigns tokenEnvVar, which shadows
// the constant the argument reads, and a file that imports os twice, which
// reaches os.LookupEnv by two names.
//
// It reads the source, not the linted tree: a case is a string, so the pin is a
// fixture (TestLookupEnvProblems).
func lookupEnvProblems(rel string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	names, positions := importedNames(f, "os")
	if len(names) == 0 {
		return []string{fmt.Sprintf("%s: imports no package os, so nothing reads %s through os.LookupEnv for run", rel, envVarName)}, nil
	}

	var problems []string
	if len(names) > 1 {
		problems = append(problems, fmt.Sprintf("%s:%d: the file imports os %d times; a composition root imports it once, so the token source's one os.LookupEnv reference has one name", rel, fset.Position(positions[len(positions)-1]).Line, len(names)))
	}

	// named is every local name the file reaches package os by: a dot import
	// hides os.LookupEnv behind a bare name, which no selector check can
	// follow, and a blank import makes no reference possible, so neither is
	// one.
	named := make([]string, 0, len(names))
	dotted := false
	for i, name := range names {
		switch name {
		case ".":
			dotted = true
			problems = append(problems, fmt.Sprintf("%s:%d: the file dot-imports os, which hides whether os.LookupEnv is the one call the token source makes; import it as os", rel, fset.Position(positions[i]).Line))
		case "_":
		default:
			named = append(named, name)
		}
	}
	if dotted {
		return problems, nil
	}

	for _, pos := range shadowsOf(f, "run") {
		problems = append(problems, fmt.Sprintf("%s:%d: run is declared or assigned here, so the call in this file is no call of the package's run; the token source is an argument of the run cmd/shift2eu-server/run.go declares", rel, fset.Position(pos).Line))
	}
	for _, pos := range shadowsOf(f, envVarIdent) {
		problems = append(problems, fmt.Sprintf("%s:%d: %s is declared or assigned here, so the identifier the token source passes os.LookupEnv is no longer the constant cmd/shift2eu-server/token.go declares; a composition root names that constant and declares nothing of its own", rel, fset.Position(pos).Line, envVarIdent))
	}

	refs, calls, sources := lookupEnvRefs(f, named)

	switch {
	case len(refs) == 0:
		problems = append(problems, fmt.Sprintf("%s: no reference to os.LookupEnv; main passes run a token source that reads %s with it (ADR 0019)", rel, envVarName))
	case len(refs) > 1:
		for i, ref := range refs {
			problems = append(problems, fmt.Sprintf("%s:%d: os.LookupEnv is reference %d of %d in this file; a composition root holds exactly one, in the token source it passes to run", rel, fset.Position(ref.Pos()).Line, i+1, len(refs)))
		}
	default:
		if problem := lookupEnvRefProblem(rel, fset, refs[0], calls, sources); problem != "" {
			problems = append(problems, problem)
		}
	}

	return problems, nil
}

// lookupEnvRefs returns every reference to os.LookupEnv in f, through any of
// pkgs, the local names the file gives package os; the call each reference is
// the function of, when it is one; and the position of each reference that is
// the sole return expression of a func literal passed directly to run
// (tokenSourceOfArg).
func lookupEnvRefs(f *ast.File, pkgs []string) (refs []*ast.SelectorExpr, calls map[token.Pos]*ast.CallExpr, sources map[token.Pos]bool) {
	calls = map[token.Pos]*ast.CallExpr{}
	sources = map[token.Pos]bool{}

	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if _, named := anyName(n.X, pkgs); named && n.Sel.Name == "LookupEnv" {
				refs = append(refs, n)
			}
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
				if _, named := anyName(sel.X, pkgs); named && sel.Sel.Name == "LookupEnv" {
					calls[sel.Pos()] = n
				}
			}

			if !isName(n.Fun, "run") {
				break
			}

			for _, arg := range n.Args {
				if source, ok := tokenSourceOfArg(arg, pkgs); ok {
					sources[source] = true
				}
			}
		}

		return true
	})

	return refs, calls, sources
}

// tokenSourceOfArg returns the position of the os.LookupEnv call that arg is the
// sole return expression of, when arg is a token source that shape: a func
// literal whose body is one return, of which this call is the only expression,
// and whose one argument is the identifier tokenEnvVar. run takes the source as
// one value with no name parameter, so the literal cannot be handed another name
// and this is the only shape the exception covers.
func tokenSourceOfArg(arg ast.Expr, pkgs []string) (token.Pos, bool) {
	lit, ok := arg.(*ast.FuncLit)
	if !ok || len(lit.Body.List) != 1 {
		return token.NoPos, false
	}

	ret, ok := lit.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return token.NoPos, false
	}

	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !isName(call.Args[0], envVarIdent) {
		return token.NoPos, false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return token.NoPos, false
	}
	if _, named := anyName(sel.X, pkgs); !named || sel.Sel.Name != "LookupEnv" {
		return token.NoPos, false
	}

	return sel.Pos(), true
}

// lookupEnvRefProblem returns what is wrong with ref, the file's one reference to
// os.LookupEnv, or "" when it is the token source run takes. Each rule is the one
// the exception of ADR 0019 has to keep: os.LookupEnv is called, with one
// argument, the identifier tokenEnvVar, and the call is the sole return
// expression of a func literal passed directly to run.
func lookupEnvRefProblem(rel string, fset *token.FileSet, ref *ast.SelectorExpr,
	calls map[token.Pos]*ast.CallExpr, sources map[token.Pos]bool,
) string {
	line := fset.Position(ref.Pos()).Line

	call, called := calls[ref.Pos()]

	switch {
	case !called:
		return fmt.Sprintf("%s:%d: os.LookupEnv is not called here; the composition root's one reference is the call its token source returns, never the function itself, an assignment or another function's argument", rel, line)
	case len(call.Args) != 1:
		return fmt.Sprintf("%s:%d: os.LookupEnv is called with %d arguments; the token source reads the identifier %s and nothing else", rel, line, len(call.Args), envVarIdent)
	case !isName(call.Args[0], envVarIdent):
		return fmt.Sprintf("%s:%d: os.LookupEnv is called with %s; the token source reads the identifier %s, which holds %s, and no other name or literal", rel, line, exprText(call.Args[0]), envVarIdent, envVarName)
	case !sources[ref.Pos()]:
		return fmt.Sprintf("%s:%d: os.LookupEnv is no token source: run takes a func() (string, bool) whose sole return expression is this call, passed to run directly (ADR 0019)", rel, line)
	}

	return ""
}

// shadowsOf returns the position of every declaration or assignment of any of
// names in f: a func, a type, a var or a const, a short variable declaration or
// another assignment, a parameter or a result of a function or of a func
// literal, and a range variable. Each one makes a reference to the name in this
// file a reference to something other than the package's declaration, so a
// check that reads a name on the AST reads f's own, not the one the package
// declares in another file.
func shadowsOf(f *ast.File, names ...string) []token.Pos {
	var positions []token.Pos

	shadow := func(id *ast.Ident) {
		if id != nil && slices.Contains(names, id.Name) {
			positions = append(positions, id.Pos())
		}
	}

	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			shadow(n.Name)
		case *ast.TypeSpec:
			shadow(n.Name)
		case *ast.ValueSpec:
			for _, name := range n.Names {
				shadow(name)
			}
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					shadow(id)
				}
			}
		case *ast.FuncType:
			shadowFields(n.Params, shadow)
			shadowFields(n.Results, shadow)
		case *ast.RangeStmt:
			if id, ok := n.Key.(*ast.Ident); ok {
				shadow(id)
			}
			if id, ok := n.Value.(*ast.Ident); ok {
				shadow(id)
			}
		}

		return true
	})

	return positions
}

// shadowFields passes the name of every named parameter or result of a func's
// signature to shadow, for the signatures that declare a name no body does.
func shadowFields(fields *ast.FieldList, shadow func(*ast.Ident)) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			shadow(name)
		}
	}
}

// exprText renders expr as the source writes it, without its positions, for a
// message about the value a composition root passed instead of tokenEnvVar.
func exprText(expr ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), expr); err != nil {
		return "an expression this check cannot render"
	}

	return buf.String()
}

// importedNames returns every local name f gives the package at path, with the
// position of each import, in source order, and nil when f imports no such
// package. A file may import one path twice, under two names: then every name
// is a way to reach the package, and a check that reads only the first one
// leaves the second unchecked. A dot import is returned as ".", a blank import
// as "_".
func importedNames(f *ast.File, path string) (names []string, positions []token.Pos) {
	for _, imp := range f.Imports {
		if imp.Path.Value != strconv.Quote(path) {
			continue
		}

		name := path
		if imp.Name != nil {
			name = imp.Name.Name
		}

		names = append(names, name)
		positions = append(positions, imp.Pos())
	}

	return names, positions
}

// anyName returns the name of the identifier expr, when expr is an identifier
// and one of names, the local names a file gives one imported package.
func anyName(expr ast.Expr, names []string) (string, bool) {
	id, ok := expr.(*ast.Ident)
	if !ok || !slices.Contains(names, id.Name) {
		return "", false
	}

	return id.Name, true
}

// isName reports whether expr is the identifier name, as os.LookupEnv writes the
// package os and run(os.LookupEnv) writes the function run.
func isName(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}

// tokenEnvVarShadow is the problem lookupEnvProblems reports at line of rel for
// a declaration or assignment of tokenEnvVar: the identifier the token source
// passes os.LookupEnv then names that declaration, not the constant
// cmd/shift2eu-server/token.go declares.
func tokenEnvVarShadow(rel string, line int) string {
	return fmt.Sprintf("%s:%d: %s is declared or assigned here, so the identifier the token source passes os.LookupEnv is no longer the constant cmd/shift2eu-server/token.go declares; a composition root names that constant and declares nothing of its own", rel, line, envVarIdent)
}

// TestLookupEnvProblems pins lookupEnvProblems, the seam of
// TestCompositionRootPassesLookupEnvToRun: the one shape ADR 0019 allows, the
// token source run takes, and every shape it refuses, the old one that handed
// os.LookupEnv itself to run included.
//
// No fixture declares tokenEnvVar: the constant lives in
// cmd/shift2eu-server/token.go, another file of the package, and the check
// holds main.go to naming it and to declaring nothing of its own. The fixtures
// that name it in a local scope pin that too (shadowsOf).
func TestLookupEnvProblems(t *testing.T) {
	t.Parallel()

	const rel = "cmd/shift2eu-server/main.go"
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			// The shape ADR 0019 has: run takes func() (string, bool), and
			// the literal's one call reads the identifier tokenEnvVar.
			name: "the token source run takes",
			src: `package main

import "os"

func main() {
	run(func() (string, bool) { return os.LookupEnv(tokenEnvVar) })
}
`,
		},
		{
			name: "an aliased import is the same package",
			src: `package main

import stdos "os"

func main() {
	run(func() (string, bool) { return stdos.LookupEnv(tokenEnvVar) })
}
`,
		},
		{
			// The old form: run took the function itself, so the name it
			// read was any caller's.
			name: "the function itself passed to run is refused",
			src: `package main

import "os"

func main() { run(os.LookupEnv) }
`,
			want: []string{
				rel + ":5: os.LookupEnv is not called here; the composition root's one reference is the call its token source returns, never the function itself, an assignment or another function's argument",
			},
		},
		{
			// The pin: the exception is one call, and a secret read beside it
			// would lint clean.
			name: "a second use is refused",
			src: `package main

import "os"

func main() {
	run(func() (string, bool) { return os.LookupEnv(tokenEnvVar) })
	_, _ = os.LookupEnv("AWS_SECRET_ACCESS_KEY")
}
`,
			want: []string{
				rel + ":6: os.LookupEnv is reference 1 of 2 in this file; a composition root holds exactly one, in the token source it passes to run",
				rel + ":7: os.LookupEnv is reference 2 of 2 in this file; a composition root holds exactly one, in the token source it passes to run",
			},
		},
		{
			// The token source has no name parameter: another variable's name
			// is refused, not passed through.
			name: "another variable's name is refused",
			src: `package main

import "os"

func main() {
	run(func() (string, bool) { return os.LookupEnv("AWS_SECRET_ACCESS_KEY") })
}
`,
			want: []string{
				rel + ":6: os.LookupEnv is called with \"AWS_SECRET_ACCESS_KEY\"; the token source reads the identifier tokenEnvVar, which holds SHIFT2EU_SERVER_TOKEN, and no other name or literal",
			},
		},
		{
			name: "the variable's name as a literal is refused too",
			src: `package main

import "os"

func main() {
	run(func() (string, bool) { return os.LookupEnv("SHIFT2EU_SERVER_TOKEN") })
}
`,
			want: []string{
				rel + ":6: os.LookupEnv is called with \"SHIFT2EU_SERVER_TOKEN\"; the token source reads the identifier tokenEnvVar, which holds SHIFT2EU_SERVER_TOKEN, and no other name or literal",
			},
		},
		{
			name: "a call with no argument is refused",
			src: `package main

import "os"

func main() {
	run(func() (string, bool) { return os.LookupEnv() })
}
`,
			want: []string{
				rel + ":6: os.LookupEnv is called with 0 arguments; the token source reads the identifier tokenEnvVar and nothing else",
			},
		},
		{
			name: "a lookup assigned to a variable is refused",
			src: `package main

import "os"

func main() {
	lookup := os.LookupEnv
	_ = lookup
}
`,
			want: []string{
				rel + ":6: os.LookupEnv is not called here; the composition root's one reference is the call its token source returns, never the function itself, an assignment or another function's argument",
			},
		},
		{
			name: "a lookup another function wraps is refused",
			src: `package main

import "os"

func main() { run(decorate(os.LookupEnv)) }

func decorate(lookup func() (string, bool)) func() (string, bool) { return lookup }
`,
			want: []string{
				rel + ":5: os.LookupEnv is not called here; the composition root's one reference is the call its token source returns, never the function itself, an assignment or another function's argument",
			},
		},
		{
			name: "a call that is not the sole return of the source is refused",
			src: `package main

import "os"

func main() {
	run(func() (string, bool) {
		value, ok := os.LookupEnv(tokenEnvVar)
		return value, ok
	})
}
`,
			want: []string{
				rel + ":7: os.LookupEnv is no token source: run takes a func() (string, bool) whose sole return expression is this call, passed to run directly (ADR 0019)",
			},
		},
		{
			name: "a source the file calls itself is refused",
			src: `package main

import "os"

func main() {
	lookup := func() (string, bool) { return os.LookupEnv(tokenEnvVar) }
	_ = lookup
}
`,
			want: []string{
				rel + ":6: os.LookupEnv is no token source: run takes a func() (string, bool) whose sole return expression is this call, passed to run directly (ADR 0019)",
			},
		},
		{
			// A local run is no call of the package's run, so the source
			// beside it is no argument of run at all.
			name: "a local run is refused",
			src: `package main

import "os"

func main() {
	run := func(lookup func() (string, bool)) {}
	run(func() (string, bool) { return os.LookupEnv(tokenEnvVar) })
}
`,
			want: []string{
				rel + ":6: run is declared or assigned here, so the call in this file is no call of the package's run; the token source is an argument of the run cmd/shift2eu-server/run.go declares",
			},
		},
		{
			name: "a package-level run is refused",
			src: `package main

import "os"

var run = func(lookup func() (string, bool)) {}

func main() {
	run(func() (string, bool) { return os.LookupEnv(tokenEnvVar) })
}
`,
			want: []string{
				rel + ":5: run is declared or assigned here, so the call in this file is no call of the package's run; the token source is an argument of the run cmd/shift2eu-server/run.go declares",
			},
		},
		{
			// The pin: the exception names a variable, and a local of
			// that name is a different one, so the source reads whatever
			// the local holds.
			name: "a local of the source's name is refused",
			src: `package main

import "os"

func main() {
	tokenEnvVar := "AWS_PROFILE"
	run(func() (string, bool) { return os.LookupEnv(tokenEnvVar) })
}
`,
			want: []string{tokenEnvVarShadow(rel, 6)},
		},
		{
			name: "a package-level variable of that name is refused",
			src: `package main

import "os"

var tokenEnvVar = "AWS_PROFILE"

func main() {
	run(func() (string, bool) { return os.LookupEnv(tokenEnvVar) })
}
`,
			want: []string{tokenEnvVarShadow(rel, 5)},
		},
		{
			name: "a function of that name is refused",
			src: `package main

import "os"

func tokenEnvVar() string { return "AWS_PROFILE" }

func main() {
	run(func() (string, bool) { return os.LookupEnv(tokenEnvVar) })
}
`,
			want: []string{tokenEnvVarShadow(rel, 5)},
		},
		{
			name: "a type of that name is refused",
			src: `package main

import "os"

func main() {
	type tokenEnvVar string
	run(func() (string, bool) { return os.LookupEnv(tokenEnvVar) })
}
`,
			want: []string{tokenEnvVarShadow(rel, 6)},
		},
		{
			name: "a parameter of that name is refused",
			src: `package main

import "os"

func configure(tokenEnvVar string) string { return tokenEnvVar }

func main() {
	run(func() (string, bool) { return os.LookupEnv(tokenEnvVar) })
}
`,
			want: []string{tokenEnvVarShadow(rel, 5)},
		},
		{
			name: "a func literal's parameter of that name is refused",
			src: `package main

import "os"

func main() {
	run(func(tokenEnvVar string) (string, bool) { return os.LookupEnv(tokenEnvVar) })
}
`,
			want: []string{tokenEnvVarShadow(rel, 6)},
		},
		{
			name: "a range variable of that name is refused",
			src: `package main

import "os"

func main() {
	for _, tokenEnvVar := range os.Environ() {
		_ = tokenEnvVar
	}
	run(func() (string, bool) { return os.LookupEnv(tokenEnvVar) })
}
`,
			want: []string{tokenEnvVarShadow(rel, 6)},
		},
		{
			// The pin: a second name for the package is a second way to
			// reach os.LookupEnv, and staticcheck's ST1019 is a nolint
			// away.
			name: "the package imported twice under two names is refused",
			src: `package main

import (
	"os"
	o "os"
)

func main() {
	run(func() (string, bool) { return o.LookupEnv(tokenEnvVar) })
}
`,
			want: []string{
				rel + ":5: the file imports os 2 times; a composition root imports it once, so the token source's one os.LookupEnv reference has one name",
			},
		},
		{
			name: "a file that reads nothing is refused",
			src: `package main

import "os"

func main() { os.Exit(0) }
`,
			want: []string{
				rel + ": no reference to os.LookupEnv; main passes run a token source that reads SHIFT2EU_SERVER_TOKEN with it (ADR 0019)",
			},
		},
		{
			name: "a dot import of os is refused",
			src: `package main

import . "os"

func main() { run(LookupEnv) }
`,
			want: []string{
				rel + ":3: the file dot-imports os, which hides whether os.LookupEnv is the one call the token source makes; import it as os",
			},
		},
		{
			name: "a file that imports no os is refused",
			src: `package main

func main() { run(nil) }
`,
			want: []string{
				rel + ": imports no package os, so nothing reads SHIFT2EU_SERVER_TOKEN through os.LookupEnv for run",
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
// beyond a check of literals. What keeps os out of the layers below the
// composition roots is depguard's domain-is-pure and app-uses-ports-not-adapters;
// internal/cli allows $gostd, os included, so neither this check nor depguard
// holds it.
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
// literal, so a fixture is what proves the check would refuse one. A case's rel
// is the file the literal is read in: a root's own Go file, or its _test.go,
// which the check holds alike.
func TestEnvironmentPathProblems(t *testing.T) {
	t.Parallel()

	const rel = "cmd/shift2eu-server/read.go"
	tests := []struct {
		name string
		rel  string
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
			rel:  "cmd/shift2eu-server/read_test.go",
			src:  "package main\n\nimport \"os\"\n\nvar _ = os.ReadFile(\"/proc/1/environ\")\n",
			want: []string{
				"cmd/shift2eu-server/read_test.go:5: the string literal \"/proc/1/environ\" holds \"/proc/\", which names every environment variable of this process, credentials included; a composition root reads the named variable it needs and nothing else",
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

			at := tt.rel
			if at == "" {
				at = rel
			}

			got, err := environmentPathProblems(at, []byte(tt.src))
			if err != nil {
				t.Fatalf("environmentPathProblems(): %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("environmentPathProblems() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
