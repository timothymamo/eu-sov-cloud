package execenv

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
)

// Finding is one place in a source file that breaks a rule of this package.
type Finding struct {
	// File is the file name the caller passed, unchanged.
	File string
	// Line is the 1-based line a reader fixes.
	Line int
	// Msg says what is wrong and how to fix it.
	Msg string
}

// runMethods are the methods of *exec.Cmd that start the child.
var runMethods = map[string]bool{
	"Run":            true,
	"Start":          true,
	"Output":         true,
	"CombinedOutput": true,
}

// environPath matches a string that is a /proc/<something>/environ path, with
// or without its leading slash (os.DirFS("/") opens it without one). A string
// that only holds such a path among other text, such as a probe source or an
// expected message in another rule's test, names no file and passes.
var environPath = regexp.MustCompile(`^/?proc/[^/]+/environ$`)

// CheckExecEnv returns a finding, in source order, for every Run, Start,
// Output or CombinedOutput call in the Go source src on a command built by
// exec.Command or exec.CommandContext whose Env was not assigned between its
// construction and that call, in the same function. exempt is handed the
// command name when the source writes it as a string literal; a command it
// admits is let through, and a name that is no literal is never exempt.
// filename names the file in the findings and in the parse error.
func CheckExecEnv(filename string, src []byte, exempt func(name string) bool) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	c := &execChecker{
		fset:     fset,
		filename: filename,
		exec:     execImportName(file),
		exempt:   exempt,
	}
	if c.exec == "" {
		return nil, nil
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			c.checkBody(fn.Body)
		}
	}
	// A function literal outside every function body, in a package-level
	// var, is a function of its own too.
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			return false
		case *ast.FuncLit:
			c.checkBody(n.Body)
			return false
		}
		return true
	})
	return c.findings, nil
}

// execChecker holds what CheckExecEnv needs while it walks one file.
type execChecker struct {
	fset     *token.FileSet
	filename string
	// exec is the name the file imports os/exec under.
	exec     string
	exempt   func(string) bool
	findings []Finding
}

// command is what the checker knows of a variable that holds an *exec.Cmd
// built in the function being checked.
type command struct {
	// exempt is whether the command's name is a literal exempt admits.
	exempt bool
	// envSet is whether Env was assigned since the command was built.
	envSet bool
}

// checkBody checks one function body. A function literal inside it is a
// function of its own, checked with no knowledge of the enclosing one's
// commands.
func (c *execChecker) checkBody(body *ast.BlockStmt) {
	commands := map[string]*command{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			c.checkBody(n.Body)
			return false
		case *ast.AssignStmt:
			c.assign(commands, n.Lhs, n.Rhs)
		case *ast.ValueSpec:
			lhs := make([]ast.Expr, len(n.Names))
			for i, name := range n.Names {
				lhs[i] = name
			}
			c.assign(commands, lhs, n.Values)
		case *ast.CallExpr:
			c.call(commands, n)
		}
		return true
	})
}

// assign records the commands an assignment builds, and the Env fields it
// sets.
func (c *execChecker) assign(commands map[string]*command, lhs, rhs []ast.Expr) {
	if len(lhs) != len(rhs) {
		return
	}
	for i, target := range lhs {
		switch target := target.(type) {
		case *ast.Ident:
			if call, ok := rhs[i].(*ast.CallExpr); ok {
				if exempt, ok := c.construction(call); ok {
					commands[target.Name] = &command{exempt: exempt}
					continue
				}
			}
			// The name now holds something else, which this checker does
			// not follow.
			delete(commands, target.Name)
		case *ast.SelectorExpr:
			if target.Sel.Name != "Env" {
				continue
			}
			if id, ok := target.X.(*ast.Ident); ok {
				if cmd, ok := commands[id.Name]; ok {
					cmd.envSet = true
				}
			}
		}
	}
}

// call reports a Run, Start, Output or CombinedOutput call on a command with
// no Env of its own.
func (c *execChecker) call(commands map[string]*command, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !runMethods[sel.Sel.Name] {
		return
	}
	switch x := sel.X.(type) {
	case *ast.Ident:
		cmd, ok := commands[x.Name]
		if !ok || cmd.exempt || cmd.envSet {
			return
		}
		c.report(sel.Sel.Pos(), fmt.Sprintf(
			"%s.%s runs a command whose Env is nil, which inherits the whole environment of the test process; assign %s.Env before the call",
			x.Name, sel.Sel.Name, x.Name))
	case *ast.CallExpr:
		exempt, ok := c.construction(x)
		if !ok || exempt {
			return
		}
		c.report(sel.Sel.Pos(), fmt.Sprintf(
			"%s is called on a command built in the same expression, whose Env is nil and inherits the whole environment of the test process; bind the command to a variable and assign its Env first",
			sel.Sel.Name))
	}
}

// construction reports whether call is exec.Command or exec.CommandContext,
// and if so whether the command it builds is exempt.
func (c *execChecker) construction(call *ast.CallExpr) (exempt, ok bool) {
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return false, false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	if !isIdent || pkg.Name != c.exec {
		return false, false
	}
	var nameArg int
	switch sel.Sel.Name {
	case "Command":
		nameArg = 0
	case "CommandContext":
		nameArg = 1
	default:
		return false, false
	}
	if len(call.Args) <= nameArg {
		return false, true
	}
	lit, isLit := call.Args[nameArg].(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		return false, true
	}
	name, err := strconv.Unquote(lit.Value)
	if err != nil {
		return false, true
	}
	return c.exempt(name), true
}

func (c *execChecker) report(pos token.Pos, msg string) {
	c.findings = append(c.findings, Finding{
		File: c.filename,
		Line: c.fset.Position(pos).Line,
		Msg:  msg,
	})
}

// execImportName returns the name file imports os/exec under, or "" when it
// does not import it (or imports it blank or dot, which builds no command
// this checker can name).
func execImportName(file *ast.File) string {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != "os/exec" {
			continue
		}
		if imp.Name == nil {
			return "exec"
		}
		if imp.Name.Name == "_" || imp.Name.Name == "." {
			return ""
		}
		return imp.Name.Name
	}
	return ""
}

// CheckEnvironLiterals returns a finding, in source order, for every string
// literal in the Go source src whose value is a /proc/<something>/environ
// path, with or without its leading slash. Comments are no literals and pass. filename names the file in the
// findings and in the parse error.
func CheckEnvironLiterals(filename string, src []byte) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			value = lit.Value
		}
		if path := environPath.FindString(value); path != "" {
			findings = append(findings, Finding{
				File: filename,
				Line: fset.Position(lit.Pos()).Line,
				Msg:  fmt.Sprintf("the literal names %s, which holds the whole environment of a process; read a named variable instead", path),
			})
		}
		return true
	})
	return findings, nil
}
